package crawler

import (
	"errors"
	"net/url"
	"sync"
)

var (
	DefaultController = NopController{}
)

// Crawler crawls web pages.
type Crawler struct {
	ctrl  Controller
	opt   *Option
	store Store

	maker     *maker
	fetcher   *fetcher
	handler   *handler
	scheduler *scheduler

	quit chan struct{}
	wg   sync.WaitGroup
}

// NewCrawler creates a new crawler.
func NewCrawler(opt *Option, store Store, ctrl Controller) *Crawler {
	if opt == nil {
		opt = DefaultOption
	}
	if store == nil {
		store = newMemStore()
	}
	if ctrl == nil {
		ctrl = DefaultController
	}
	cw := &Crawler{
		opt:   opt,
		store: store,
		ctrl:  ctrl,
		quit:  make(chan struct{}),
	}

	// connect each part
	cw.maker = cw.newRequestMaker()
	cw.fetcher = cw.newFetcher()
	cw.handler = cw.newRespHandler()
	cw.scheduler = cw.newScheduler()

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
func (cw *Crawler) Crawl(seeds ...string) error {
	cw.wg.Add(4)
	start(cw.maker)
	start(cw.fetcher)
	start(cw.handler)
	start(cw.scheduler)

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
		if ok, err := cw.store.PutNX(&URL{
			Loc: *u,
		}); err != nil {
			return err
		} else if ok {
			cw.scheduler.NewIn <- u
		}
	}
	return nil
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
}
