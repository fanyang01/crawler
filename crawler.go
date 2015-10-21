package crawler

import (
	"errors"
	"net/url"
	"sync"
	"time"
)

var (
	DefaultCtrler = NewMux()
)

type Statistic struct {
	Uptime time.Time
	URLs   int32
	Times  int32
	Errors int32
	Done   int32
}

// Crawler crawls web pages.
type Crawler struct {
	ctrler    Controller
	opt       *Option
	urlStore  URLStore
	maker     *maker
	fetcher   *fetcher
	reciever  *reciever
	finder    *finder
	filter    *filter
	scheduler *scheduler
	statistic Statistic
	done      chan struct{}
	wg        sync.WaitGroup
}

type quit struct {
	Done chan struct{}
	WG   *sync.WaitGroup
}

// NewCrawler creates a new crawler.
func NewCrawler(opt *Option, store URLStore, ctrler Controller) *Crawler {
	if opt == nil {
		opt = DefaultOption
	}
	if store == nil {
		store = newMemStore()
	}
	if ctrler == nil {
		ctrler = DefaultCtrler
	}
	cw := &Crawler{
		opt:      opt,
		urlStore: store,
		ctrler:   ctrler,
		done:     make(chan struct{}),
	}

	// connect each part
	cw.maker = cw.newRequestMaker()
	cw.fetcher = cw.newFetcher()
	cw.reciever = cw.newRespHandler()
	cw.finder = cw.newFinder()
	cw.filter = cw.newFilter()
	cw.scheduler = cw.newScheduler()

	cw.maker.In = cw.scheduler.Out
	cw.fetcher.In = cw.maker.Out
	cw.reciever.In = cw.fetcher.Out
	cw.finder.In = cw.reciever.Out
	cw.filter.In = cw.finder.Out
	cw.scheduler.ErrIn = cw.fetcher.ErrOut
	cw.scheduler.NewIn = cw.filter.NewOut
	cw.scheduler.AgainIn = cw.filter.AgainOut

	return cw
}

// Crawl starts the crawler using several seeds.
func (cw *Crawler) Crawl(seeds ...string) error {
	cw.wg.Add(6)
	start(cw.maker)
	start(cw.fetcher)
	start(cw.reciever)
	start(cw.finder)
	start(cw.filter)
	cw.scheduler.start()

	err := cw.addSeeds(seeds...)
	if err != nil {
		cw.Stop()
		return err
	}
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
		u.Fragment = ""
		if cw.urlStore.PutIfNonExist(&URL{
			Loc:   *u,
			Score: 1024, // Give seeds highest priority
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
	if cw.urlStore.PutIfNonExist(&URL{
		Loc:   *uu,
		Score: score,
	}) {
		cw.scheduler.NewIn <- uu
	}
}

// Stop stops the crawler.
func (cw *Crawler) Stop() {
	close(cw.done)
	cw.wg.Wait()
}
