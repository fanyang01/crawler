package crawler

import (
	"bufio"
	"errors"
	"io/ioutil"
	"net/url"
	"os"
	"sync"

	"github.com/Sirupsen/logrus"
	"github.com/fanyang01/crawler/queue"
)

// Crawler crawls web pages.
type Crawler struct {
	ctrl  Controller
	store Store
	opt   *Option
	log   *logrus.Logger

	maker     *maker
	fetcher   *fetcher
	handler   *handler
	scheduler *scheduler

	quit chan struct{}
	wg   sync.WaitGroup

	tmpfile *os.File
}

type Config struct {
	Controller Controller
	Store      Store
	Queue      queue.WaitQueue
	Logger     *logrus.Logger
	Option     *Option
}

var (
	DefaultController = NopController{}
	defaultConfig     = &Config{}
	log               *logrus.Logger
)

func initConfig(cfg *Config) *Config {
	if cfg == nil {
		cfg = defaultConfig
	}
	if cfg.Option == nil {
		cfg.Option = DefaultOption
	}
	if cfg.Store == nil {
		cfg.Store = NewMemStore()
	}
	if cfg.Queue == nil {
		cfg.Queue = NewMemQueue(64 << 10)
	}
	if cfg.Controller == nil {
		cfg.Controller = DefaultController
	}
	if cfg.Logger == nil {
		cfg.Logger = logrus.StandardLogger()
	}
	return cfg
}

// NewCrawler creates a new crawler.
func NewCrawler(cfg *Config) *Crawler {
	cfg = initConfig(cfg)
	log = cfg.Logger

	cw := &Crawler{
		opt:   cfg.Option,
		store: cfg.Store,
		ctrl:  cfg.Controller,
		log:   cfg.Logger,
		quit:  make(chan struct{}),
	}

	// connect each part
	cw.maker = cw.newRequestMaker()
	cw.fetcher = cw.newFetcher()
	cw.handler = cw.newRespHandler()
	cw.scheduler = cw.newScheduler(cfg.Queue)

	// normal flow
	cw.maker.In = cw.scheduler.Out
	cw.fetcher.In = cw.maker.Out
	cw.handler.In = cw.fetcher.Out
	cw.scheduler.ResIn = cw.handler.Out

	// additional flow
	cw.handler.DoneOut = cw.scheduler.DoneIn
	cw.fetcher.ErrOut = cw.scheduler.ErrIn

	return cw
}

// Crawl starts the crawler using several seeds.
func (cw *Crawler) Crawl(seeds ...string) (err error) {
	cw.wg.Add(4)
	start(cw.maker)
	start(cw.fetcher)
	start(cw.handler)
	start(cw.scheduler)

	nr, err := cw.recover()
	if err != nil {
		cw.Stop()
		return
	}

	ns, err := cw.addSeeds(seeds...)
	if err != nil {
		cw.Stop()
		return
	}

	if nr+ns <= 0 {
		cw.Stop()
	}
	return nil
}

func (cw *Crawler) recover() (n int, err error) {
	ps, ok := cw.store.(PersistableStore)
	if !ok {
		return
	}
	tmpfile, err := ioutil.TempFile("", "crawler")
	if err != nil {
		return
	}
	cw.tmpfile = tmpfile

	w := bufio.NewWriter(tmpfile)
	if n, err = ps.Recover(w); err != nil {
		return
	}
	w.Flush()
	if _, err = tmpfile.Seek(0, 0); err != nil {
		return
	}
	go func() {
		scanner := bufio.NewScanner(tmpfile)
		for scanner.Scan() {
			u, err := url.Parse(scanner.Text())
			if err != nil {
				log.Error(err)
				continue
			}
			// TODO
			cw.scheduler.RecoverIn <- u
		}
		if err := scanner.Err(); err != nil {
			log.Error(err)
		}
		return
	}()
	return
}

func (cw *Crawler) Wait() {
	cw.wg.Wait()
}

func (cw *Crawler) addSeeds(seeds ...string) (n int, err error) {
	if len(seeds) == 0 {
		return 0, errors.New("crawler: no seed provided")
	}
	for _, seed := range seeds {
		var u *url.URL
		var ok bool
		if u, err = url.Parse(seed); err != nil {
			return
		}
		u.Fragment = ""

		if ok, err = cw.store.PutNX(&URL{
			Loc: *u,
		}); err != nil {
			return
		} else if ok {
			cw.scheduler.NewIn <- u
			n++
		}
	}
	return
}

// Enqueue adds urls to queue.
func (cw *Crawler) Enqueue(urls ...string) error {
	for _, u := range urls {
		uu, err := url.Parse(u)
		if err != nil {
			return err
		}
		if ok, err := cw.store.PutNX(&URL{
			Loc: *uu,
		}); err != nil {
			return err
		} else if ok {
			cw.scheduler.NewIn <- uu
		}
	}
	return nil
}

// Stop stops the crawler.
func (cw *Crawler) Stop() {
	close(cw.quit)
	cw.wg.Wait()

	if cw.tmpfile != nil {
		os.Remove(cw.tmpfile.Name())
	}
}
