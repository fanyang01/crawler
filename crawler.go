package crawler

import (
	"errors"
	"net/url"
	"sync"
	"time"
)

var (
	DefaultCtrler = OnceController{}
)

// Crawler crawls web pages.
type Crawler struct {
	ctl       Controller
	opt       *Option
	urlStore  URLStore
	maker     *maker
	fetcher   *fetcher
	handler   *handler
	finder    *finder
	filter    *filter
	scheduler *scheduler
	statistic Statistic
	quit      chan struct{}
	wg        sync.WaitGroup
}

// NewCrawler creates a new crawler.
func NewCrawler(opt *Option, store URLStore, ctl Controller) *Crawler {
	if opt == nil {
		opt = DefaultOption
	}
	if store == nil {
		store = newMemStore()
	}
	if ctl == nil {
		ctl = DefaultCtrler
	}
	cw := &Crawler{
		opt:      opt,
		urlStore: store,
		ctl:      ctl,
		quit:     make(chan struct{}),
	}

	// connect each part
	cw.maker = cw.newRequestMaker()
	cw.fetcher = cw.newFetcher()
	cw.handler = cw.newRespHandler()
	cw.finder = cw.newFinder()
	cw.filter = cw.newFilter()
	cw.scheduler = cw.newScheduler()

	// normal flow
	cw.maker.In = cw.scheduler.Out
	cw.fetcher.In = cw.maker.Out
	cw.handler.In = cw.fetcher.Out
	cw.finder.In = cw.handler.Out
	cw.filter.In = cw.finder.Out
	cw.scheduler.ResIn = cw.filter.Out
	// additional flow
	cw.filter.NewOut = cw.scheduler.NewIn
	cw.handler.DoneOut = cw.scheduler.DoneIn
	cw.fetcher.ErrOut = cw.scheduler.ErrIn

	return cw
}

// Crawl starts the crawler using several seeds.
func (cw *Crawler) Crawl(seeds ...string) error {
	cw.wg.Add(6)
	start(cw.maker)
	start(cw.fetcher)
	start(cw.handler)
	start(cw.finder)
	start(cw.filter)
	cw.scheduler.start()

	cw.statistic.Uptime = time.Now()

	err := cw.addSeeds(seeds...)
	if err != nil {
		cw.Stop()
		return err
	}
	return nil
}

func (cw *Crawler) Wait() {
	cw.wg.Wait()
}

func (cw *Crawler) addSeeds(seeds ...string) error {
	if len(seeds) == 0 {
		return errors.New("crawler: no seed provided")
	}
	for _, seed := range seeds {
		u, err := url.Parse(seed)
		if err != nil {
			return err
		}
		u.Fragment = ""
		if cw.urlStore.PutNX(&URL{
			Loc: *u,
		}) {
			cw.scheduler.NewIn <- u
		}
	}
	return nil
}

// Enqueue adds a url with optional score to queue.
func (cw *Crawler) Enqueue(u string, score int64) {
	uu, err := url.Parse(u)
	if err != nil {
		return
	}
	if cw.urlStore.PutNX(&URL{
		Loc: *uu,
	}) {
		cw.scheduler.NewIn <- uu
		cw.statistic.IncURL()
	}
}

// Stop stops the crawler.
func (cw *Crawler) Stop() {
	close(cw.quit)
	cw.wg.Wait()
}
