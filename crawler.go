package crawler

import (
	"errors"
	"net/url"
	"sync"

	"github.com/fanyang01/crawler/urlx"
	"github.com/inconshreveable/log15"
)

// Crawler crawls web pages.
type Crawler struct {
	ctrl   Controller
	store  Store
	opt    *Option
	logger log15.Logger

	maker     *maker
	fetcher   *fetcher
	handler   *handler
	scheduler *scheduler

	normalize func(*url.URL) error

	quit chan struct{}
	wg   sync.WaitGroup
}

// NewCrawler creates a new crawler.
func NewCrawler(cfg *Config) *Crawler {
	cfg = initConfig(cfg)
	cw := &Crawler{
		opt:       cfg.Option,
		store:     storeWrapper{cfg.Store},
		ctrl:      cfg.Controller,
		logger:    cfg.Logger,
		normalize: cfg.NormalizeURL,
		quit:      make(chan struct{}),
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
	cw.scheduler.In = cw.handler.Out

	// additional flow
	cw.maker.ErrOut = cw.scheduler.ErrIn
	cw.fetcher.RetryOut = cw.scheduler.RetryIn
	cw.handler.ErrOut = cw.scheduler.ErrIn
	cw.handler.RetryOut = cw.scheduler.RetryIn

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
		cw.logger.Error("failed to recover from persistent storage", "err", err)
		cw.Stop()
		return
	}

	ns, err := cw.addSeeds(seeds...)
	if err != nil {
		cw.logger.Error("add seeds", "err", err)
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
	var (
		ch    = make(chan *url.URL, 1024)
		chErr = make(chan error, 1)
		cnt   int
	)
	go func() {
		err := ps.Recover(ch)
		close(ch)
		chErr <- err
	}()
	for u := range ch {
		cw.scheduler.RecoverIn <- u
		cnt++
	}
	return cnt, <-chErr
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
		if u, err = urlx.Parse(seed, cw.normalize); err != nil {
			return
		}
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
		uu, err := urlx.Parse(u, cw.normalize)
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
}

func (cw *Crawler) Logger() log15.Logger { return cw.logger }
