package crawler

import (
	"errors"
	"log"
	"net/url"
)

type Crawler struct {
	ctrl        Controller
	option      *Option
	queue       *urlHeap
	pool        *Pool
	handler     *respHandler
	filter      *filter
	constructor *requestConstructor
	parser      *linkParser
	sites       map[string]*Site
}

type Ctrl struct{}

func (c Ctrl) HandleResponse(resp *Response) { log.Println(resp.Locations) }
func (c Ctrl) Score(u *url.URL) float64      { return 0.5 }

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
	return &Crawler{
		option:      opt,
		queue:       newURLQueue(),
		handler:     newRespHandler(opt),
		pool:        newPool(opt),
		filter:      newFilter(opt),
		constructor: newRequestConstructor(opt),
		parser:      newLinkParser(opt),
		sites:       make(map[string]*Site),
	}
}

func siteRoot(u *url.URL) string {
	uu := url.URL{
		Scheme: u.Scheme,
		Host:   u.Host,
	}
	return uu.String()
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
		if site, err := NewSiteFromURL(u); err != nil {
			return err
		} else {
			c.sites[site.Root] = site
			uu := new(URL)
			uu.Loc = u
			c.queue.Push(uu)
		}
	}
	return nil
}

func (c *Crawler) Crawl() {
	c.pool.Serve()
	ch := make(chan *URL, c.option.PriorityQueueBufLen)
	go func() {
		for {
			ch <- c.queue.Pop()
		}
	}()

	c.constructor.In = ch
	c.constructor.Start()

	c.pool.In = c.constructor.Out
	c.pool.Start()

	c.handler.In = c.pool.Out
	c.handler.Start()

	c.parser.In = c.handler.Out
	c.parser.Start()

	go func() {
		for doc := range c.parser.Out {
			for _, u := range doc.SubURLs {
				uu := new(URL)
				uu.Loc = u
				c.queue.Push(uu)
			}
		}
	}()
}
