package crawler

import (
	"errors"
	"net/url"
	"time"
)

const (
	DefaultScore int64 = 512
)

// Crawler crawls web pages.
type Crawler struct {
	option    *Option
	urlStore  URLStore
	router    *Mux
	maker     *maker
	fetcher   *fetcher
	handler   *respHandler
	finder    *finder
	filter    *filter
	scheduler *scheduler
	stdClient *StdClient
	done      chan struct{}
	query     chan *HandlerQuery
}

// NewCrawler creates a new crawler.
func NewCrawler(store URLStore, opt *Option) *Crawler {
	if store == nil {
		store = newMemStore()
	}
	if opt == nil {
		opt = DefaultOption
	}
	cw := &Crawler{
		option:   opt,
		urlStore: store,
		done:     make(chan struct{}),
		query:    make(chan *HandlerQuery, 128),
	}
	cw.router = NewMux()

	entry := make(chan *url.URL, opt.NWorker.Scheduler)
	// connect each part
	cw.maker = newRequestMaker(
		opt.NWorker.Maker,
		entry,
		cw.done,
		cw.query)
	cw.fetcher = newFetcher(opt.NWorker.Fetcher,
		cw.maker.Out,
		cw.done,
		cw.scheduler.eQueue,
		cw.urlStore)
	cw.handler = newRespHandler(opt.NWorker.Handler,
		cw.fetcher.Out,
		cw.done,
		cw.query)
	cw.finder = newFinder(opt.NWorker.Parser,
		cw.handler.Out,
		cw.done)
	cw.filter = newFilter(opt.NWorker.Filter,
		cw.finder.Out,
		cw.done,
		cw.query,
		cw.urlStore)
	cw.scheduler = newScheduler(opt.NWorker.Scheduler,
		cw.filter.New,
		cw.filter.Fetched,
		cw.done,
		cw.query,
		cw.urlStore,
		entry)
	return cw
}

// Handle is similiar with http.Handle. It registers handler for given pattern.
func (cw *Crawler) Handle(pattern string, handler Handler) {
	cw.router.Add(pattern, handler)
}

// Crawl starts the crawler using several seeds.
func (cw *Crawler) Crawl(seeds ...string) error {
	err := cw.addSeeds(seeds...)
	if err != nil {
		return err
	}
	cw.router.Serve(cw.query)
	cw.scheduler.start()
	cw.maker.start()
	cw.fetcher.start()
	cw.handler.start()
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
func (cw *Crawler) Enqueue(u string, score ...int64) {
	uu, err := url.Parse(u)
	if err != nil {
		return
	}
	if _, ok := cw.urlStore.Get(*uu); ok {
		return
	}
	sc := DefaultScore
	if len(score) > 0 {
		sc = score[0]
	}
	cw.urlStore.Put(URL{
		Loc:   *uu,
		Score: sc,
	})
	cw.scheduler.New <- uu
}

// Stop stops the crawler.
func (cw *Crawler) Stop() {
	close(cw.done)
	time.Sleep(1E9)
}
