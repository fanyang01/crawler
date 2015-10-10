package crawler

import (
	"errors"
	"log"
	"net/url"
	"time"
)

type Crawler struct {
	option    *Option
	urlStore  URLStore
	router    *router
	maker     *maker
	fetcher   *fetcher
	handler   *handler
	finder    *finder
	filter    *filter
	scheduler *scheduler
	stdClient *StdClient
	done      chan struct{}
	query     chan *ctrlQuery
}

type Ctrl struct{}

func (c Ctrl) Handle(resp *Response) bool {
	log.Println(resp.Locations)
	return true
}
func (c Ctrl) Schedule(u URL) (score int64, at time.Time) { return 512, time.Now().Add(time.Second) }
func (c Ctrl) Accept(_ Anchor) bool                       { return true }
func (c Ctrl) SetRequest(_ *Request)                      {}

var (
	DefaultController = &Ctrl{}
)

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
		query:    make(chan *ctrlQuery, 128),
	}
	cw.router = newRouter()

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
	cw.handler = newHandler(opt.NWorker.Handler,
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

func (cw *Crawler) Handle(pattern string, ctrl Controller) {
	cw.router.Add(pattern, ctrl)
}

func (cw *Crawler) Crawl() {
	cw.router.Serve(cw.query)
	cw.scheduler.start()
	cw.maker.start()
	cw.fetcher.start()
	cw.handler.start()
	cw.finder.start()
	cw.filter.start()
}

func (cw *Crawler) AddSeeds(seeds ...string) error {
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

func (cw *Crawler) Stop() {
	close(cw.done)
	time.Sleep(1E9)
}
