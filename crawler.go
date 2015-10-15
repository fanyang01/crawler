package crawler

import (
	"errors"
	"net/url"
	"time"
)

// Crawler crawls web pages.
type Crawler struct {
	handler   Handler
	option    *Option
	urlStore  URLStore
	maker     *maker
	fetcher   *fetcher
	reciever  *reciever
	finder    *finder
	filter    *filter
	scheduler *scheduler
	stdClient *StdClient
	done      chan struct{}
}

// NewCrawler creates a new crawler.
func NewCrawler(opt *Option, handler Handler, store URLStore) *Crawler {
	if opt == nil {
		opt = DefaultOption
	}
	if store == nil {
		store = newMemStore()
	}
	if handler == nil {
		handler = DefaultMux
	}
	cw := &Crawler{
		option:   opt,
		urlStore: store,
		done:     make(chan struct{}),
	}

	entry := make(chan *url.URL, opt.NWorker.Scheduler)
	// connect each part
	cw.maker = newRequestMaker(
		opt.NWorker.Maker,
		entry,
		cw.done,
		cw.handler)
	cw.fetcher = newFetcher(opt.NWorker.Fetcher,
		cw.maker.Out,
		cw.done,
		cw.scheduler.eQueue,
		cw.urlStore,
		opt.MaxCacheSize)
	cw.reciever = newRespHandler(opt.NWorker.Handler,
		cw.fetcher.Out,
		cw.done,
		cw.handler)
	cw.finder = newFinder(opt.NWorker.Parser,
		cw.reciever.Out,
		cw.done)
	cw.filter = newFilter(opt.NWorker.Filter,
		cw.finder.Out,
		cw.done,
		cw.handler,
		cw.urlStore)
	cw.scheduler = newScheduler(opt.NWorker.Scheduler,
		cw.filter.New,
		cw.filter.Fetched,
		cw.done,
		cw.handler,
		cw.urlStore,
		entry)
	return cw
}

// Crawl starts the crawler using several seeds.
func (cw *Crawler) Crawl(seeds ...string) error {
	err := cw.addSeeds(seeds...)
	if err != nil {
		return err
	}
	cw.scheduler.start()
	cw.maker.start()
	cw.fetcher.start()
	cw.reciever.start()
	cw.finder.start()
	cw.filter.start()
	return nil
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
		cw.urlStore.Put(URL{
			Loc:   *u,
			Score: 1024, // Give seeds highest priority
		})
		cw.scheduler.New <- u
	}
	return nil
}

// Enqueue adds a url with optional score to queue.
func (cw *Crawler) Enqueue(u string, score int64) {
	uu, err := url.Parse(u)
	if err != nil {
		return
	}
	if _, ok := cw.urlStore.Get(*uu); ok {
		return
	}
	cw.urlStore.Put(URL{
		Loc:   *uu,
		Score: score,
	})
	cw.scheduler.New <- uu
}

// Stop stops the crawler.
func (cw *Crawler) Stop() {
	close(cw.done)
	time.Sleep(1E9)
}
