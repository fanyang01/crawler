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
	queue       *pqueue
	fetcher     *fetcher
	filter      *filter
	constructor *requestConstructor
	parser      *linkParser
	sites       sites
}

type Ctrl struct{}

func (c Ctrl) Handle(resp *Response, _ *Doc) { log.Println(resp.Locations) }
func (c Ctrl) Score(u *URL) int64            { return 512 }

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
		queue:       newPQueue(),
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
		c.queue.Push(uu)
		uu.Enqueue.Count++
		uu.Enqueue.Time = time.Now()
		c.filter.pool.Add(uu)
	}
	return nil
}

func (c *Crawler) Crawl() {
	ch := make(chan *URL, c.option.PriorityQueueBufLen)
	go func() {
		for {
			ch <- c.queue.Pop()
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
		for u := range c.filter.Out {
			c.queue.Push(u)
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
