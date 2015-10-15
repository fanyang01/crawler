package crawler

import (
	"errors"
	"net/url"
	"sync"
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
	wg        sync.WaitGroup
}

type quit struct {
	Done chan struct{}
	WG   *sync.WaitGroup
}

// NewCrawler creates a new crawler.
func NewCrawler(opt *Option, store URLStore, handler Handler) *Crawler {
	if opt == nil {
		opt = DefaultOption
	}
	if store == nil {
		store = newMemStore()
	}
	if handler == nil {
		handler = DefaultHandler
	}
	cw := &Crawler{
		option:   opt,
		urlStore: store,
		handler:  handler,
		done:     make(chan struct{}),
	}

	// connect each part
	cw.maker = newRequestMaker(
		opt.NWorker.Maker,
		cw.handler)
	cw.fetcher = newFetcher(opt.NWorker.Fetcher,
		cw.urlStore,
		opt.MaxCacheSize)
	cw.reciever = newRespHandler(opt.NWorker.Handler,
		cw.handler)
	cw.finder = newFinder(opt.NWorker.Finder)
	cw.filter = newFilter(opt.NWorker.Filter,
		cw.handler,
		cw.urlStore)
	cw.scheduler = newScheduler(opt.NWorker.Scheduler,
		cw.handler,
		cw.urlStore)

	cw.maker.In = cw.scheduler.Out
	cw.fetcher.In = cw.maker.Out
	cw.fetcher.ErrOut = cw.scheduler.ErrIn
	cw.reciever.In = cw.fetcher.Out
	cw.finder.In = cw.reciever.Out
	cw.filter.In = cw.finder.Out
	cw.scheduler.NewIn = cw.filter.NewOut
	cw.scheduler.AgainIn = cw.filter.AgainOut

	cw.maker.Done = cw.done
	cw.fetcher.Done = cw.done
	cw.reciever.Done = cw.done
	cw.finder.Done = cw.done
	cw.filter.Done = cw.done
	cw.scheduler.Done = cw.done

	cw.maker.WG = &cw.wg
	cw.fetcher.WG = &cw.wg
	cw.reciever.WG = &cw.wg
	cw.finder.WG = &cw.wg
	cw.filter.WG = &cw.wg
	cw.scheduler.WG = &cw.wg

	return cw
}

// Crawl starts the crawler using several seeds.
func (cw *Crawler) Crawl(seeds ...string) error {
	err := cw.addSeeds(seeds...)
	if err != nil {
		return err
	}
	cw.wg.Add(6)
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
		cw.scheduler.NewIn <- u
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
	cw.scheduler.NewIn <- uu
}

// Stop stops the crawler.
func (cw *Crawler) Stop() {
	close(cw.done)
	cw.wg.Wait()
}
