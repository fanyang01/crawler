package crawler

import (
	"errors"
	"log"
	"net/url"
	"sync"
	"time"
)

type sites struct {
	m map[string]*Site
	sync.RWMutex
}

func newSites() sites {
	return sites{
		m: make(map[string]*Site),
	}
}

type Crawler struct {
	seeds       []*url.URL
	ctrl        Controller
	option      *Option
	pool        *pool
	pQueue      *pqueue
	tQueue      *tqueue
	eQueue      chan url.URL
	fetcher     *fetcher
	filter      *filter
	constructor *requestMaker
	parser      *linkParser
	sites       sites
	stdClient   *StdClient
	done        chan struct{}
}

type Ctrl struct{}

func (c Ctrl) Handle(resp *Response, _ *Doc)           { log.Println(resp.Locations) }
func (c Ctrl) Score(u URL) (score int64, at time.Time) { return 512, time.Now().Add(time.Second) }
func (c Ctrl) Accept(_ url.URL) bool                   { return true }
func (c Ctrl) SetRequest(_ *Request)                   {}

var (
	DefaultController = &Ctrl{}
)

func NewCrawler(ctrl Controller, opt *Option) *Crawler {
	if ctrl == nil {
		ctrl = DefaultController
	}
	if opt == nil {
		opt = DefaultOption
	}
	cw := &Crawler{
		ctrl:   ctrl,
		option: opt,
		pool:   newPool(),
		pQueue: newPQueue(opt.PriorityQueue.MaxLen),
		tQueue: newTQueue(opt.TimeQueue.MaxLen),
		eQueue: make(chan url.URL, opt.ErrorQueueLen),
		sites:  newSites(),
		done:   make(chan struct{}),
	}
	cw.constructor = newRequestMaker(opt, ctrl)
	cw.fetcher = newFetcher(opt, cw.eQueue)
	cw.parser = newLinkParser(opt, ctrl)
	cw.filter = newFilter(opt, cw, ctrl)
	cw.constructor.Done = cw.done
	cw.fetcher.Done = cw.done
	cw.parser.Done = cw.done
	cw.filter.Done = cw.done
	return cw
}

func (c *Crawler) AddSeeds(seeds ...string) error {
	if len(seeds) == 0 {
		return errors.New("crawler: no seed provided")
	}
	for _, seed := range seeds {
		u, err := url.Parse(seed)
		if err != nil {
			return err
		}
		uu := newURL(*u)
		c.pQueue.Push(uu)
	}
	return nil
}

func (c *Crawler) Crawl() {
	// Move available URL to priority queue from time queue
	go func() {
		duration := 100 * time.Millisecond
		for {
			if !c.tQueue.IsAvailable() {
				time.Sleep(duration)
				duration = duration * 2
				continue
			}
			if urls, ok := c.tQueue.MultiPop(); ok {
				for _, u := range urls {
					c.pQueue.Push(u)
				}
				duration = 100 * time.Millisecond
			}
		}
	}()

	// Pop URL from priority queue
	ch := make(chan url.URL, c.option.PriorityQueue.BufLen)
	go func() {
		for {
			select {
			case ch <- c.pQueue.Pop():
			case <-c.done:
				close(ch)
				return
			}
		}
	}()

	c.constructor.In = ch
	c.constructor.Start()

	c.fetcher.In = c.constructor.Out
	c.fetcher.Start()

	c.parser.In = c.fetcher.Out
	c.parser.Start()

	c.filter.In = c.parser.Out
	c.filter.Start()

	// Push output of filter into queue
	go func() {
		for {
			select {
			case u := <-c.filter.Out:
				// WARNING: don't use address of u, for u is reused.
				uu := u
				if u.nextTime.After(time.Now()) {
					c.tQueue.Push(&uu)
				} else {
					c.pQueue.Push(&uu)
				}
			case <-c.done:
				return
			}
		}
	}()

	// Periodically retry urls in error queue
	go func() {
		for {
			select {
			case u := <-c.eQueue:
				uu := newURL(u)
				uu.nextTime = time.Now().Add(c.option.RetryDelay)
				c.tQueue.Push(uu)
			case <-c.done:
				return
			}
		}
	}()
}

func (cw *Crawler) Stop() {
	close(cw.done)
}
