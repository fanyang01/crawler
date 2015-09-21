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
	ctrl        Controller
	option      *Option
	pool        *pool
	pQueue      *pqueue
	tQueue      *tqueue
	fetcher     *fetcher
	filter      *filter
	constructor *requestConstructor
	parser      *linkParser
	sites       sites
}

type Ctrl struct{}

func (c Ctrl) Handle(resp *Response, _ *Doc)            { log.Println(resp.Locations) }
func (c Ctrl) Score(u *URL) (score int64, at time.Time) { return 512, time.Now().Add(time.Second) }

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
		ctrl:        ctrl,
		option:      opt,
		pool:        newPool(),
		pQueue:      newPQueue(opt.PriorityQueue.MaxLen),
		tQueue:      newTQueue(opt.TimeQueue.MaxLen),
		fetcher:     newFetcher(opt),
		constructor: newRequestConstructor(opt),
		parser:      newLinkParser(opt),
		sites:       newSites(),
	}
	cw.filter = newFilter(cw, opt)
	return cw
}

func (c *Crawler) Begin(seeds ...string) error {
	if len(seeds) == 0 {
		return errors.New("crawler: no seed provided")
	}
	for _, seed := range seeds {
		u, err := url.Parse(seed)
		if err != nil {
			return err
		}
		if err := c.addSite(u); err != nil {
			log.Println(err)
			continue
		}
		if !c.testRobot(u) {
			continue
		}
		uu := newURL(*u)
		uu.Priority = 1.0
		uu.processing = true
		c.pool.Lock()
		c.pool.Add(uu)
		c.pool.Unlock()
		c.pQueue.Push(uu)
	}
	return nil
}

func (c *Crawler) Crawl() {
	ch := make(chan *URL, c.option.PriorityQueue.BufLen)
	go func() {
		for {
			ch <- c.pQueue.Pop()
		}
	}()

	c.constructor.In = ch
	c.constructor.Start()

	c.fetcher.In = c.constructor.Out
	c.fetcher.Start()

	c.parser.In = c.fetcher.Out
	c.parser.Start(c.ctrl)

	c.filter.In = c.parser.Out
	c.filter.Start(c.ctrl)

	go func() {
		duration := time.Second
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
				duration = time.Second
			}
		}
	}()

	go func() {
		for u := range c.filter.Out {
			if u.nextTime.After(time.Now()) {
				c.tQueue.Push(u)
			} else {
				c.pQueue.Push(u)
			}
		}
	}()
}

func siteRoot(u *url.URL) string {
	uu := url.URL{
		Scheme: u.Scheme,
		Host:   u.Host,
	}
	return uu.String()
}

func (cw *Crawler) addSite(u *url.URL) error {
	root := siteRoot(u)
	cw.sites.Lock()
	defer cw.sites.Unlock()
	site, ok := cw.sites.m[root]
	if ok {
		return nil
	}
	var err error
	site, err = NewSite(root)
	if err != nil {
		return err
	}
	if err := site.FetchRobots(); err != nil {
		return err
	}
	site.FetchSitemap()
	cw.pool.Lock()
	for _, u := range site.Map.URLSet {
		uu, ok := cw.pool.Get(u.Loc)
		if !ok {
			uu = newURL(u.Loc)
		}
		if uu.processing {
			continue
		}
		uu.processing = true
		cw.pool.Add(uu)
		cw.pQueue.Push(uu)
	}
	cw.pool.Unlock()
	cw.sites.m[root] = site
	return nil
}

func (cw *Crawler) testRobot(u *url.URL) bool {
	cw.sites.RLock()
	defer cw.sites.RUnlock()
	site, ok := cw.sites.m[siteRoot(u)]
	if !ok {
		return false
	}
	return site.Robot.TestAgent(u.Path, cw.option.RobotoAgent)
}
