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
	processing  int
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
		parser: newLinkParser(opt),
		sites:  newSites(),
	}
	cw.constructor = newRequestMaker(cw, opt)
	cw.fetcher = newFetcher(cw, opt)
	cw.filter = newFilter(cw, opt)
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
	exit := make(chan struct{})
	go func() {
		for {
			ch <- c.pQueue.Pop()
		}
		close(ch)
		exit <- struct{}{}
	}()

	c.constructor.In = ch
	c.constructor.Start(c.ctrl)

	c.fetcher.In = c.constructor.Out
	c.fetcher.Start()

	c.parser.In = c.fetcher.Out
	c.parser.Start(c.ctrl)

	c.filter.In = c.parser.Out
	c.filter.Start(c.ctrl)

	// Push output of filter into queue
	go func() {
		for u := range c.filter.Out {
			// WARNING: don't use address of u, for u is reused.
			uu := u
			if u.nextTime.After(time.Now()) {
				c.tQueue.Push(&uu)
			} else {
				c.pQueue.Push(&uu)
			}
		}
	}()

	// Periodically retry urls in error queue
	go func() {
		for u := range c.eQueue {
			uu := newURL(u)
			uu.nextTime = time.Now().Add(c.option.RetryDelay)
			c.tQueue.Push(uu)
		}
	}()
	// <-exit
}
