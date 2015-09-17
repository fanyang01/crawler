package crawler

import (
	"errors"
	"log"
	"net/url"
	"time"
)

type Crawler struct {
	ctrl        Controller
	option      *Option
	queue       *urlHeap
	fetcher     *fetcher
	handler     *respHandler
	filter      *filter
	constructor *requestConstructor
	parser      *linkParser
}

type Ctrl struct{}

func (c Ctrl) HandleResponse(resp *Response) { log.Println(resp.Locations) }
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
	return &Crawler{
		ctrl:        ctrl,
		option:      opt,
		queue:       newURLQueue(),
		handler:     newRespHandler(opt),
		fetcher:     newFetcher(opt),
		filter:      newFilter(opt),
		constructor: newRequestConstructor(opt),
		parser:      newLinkParser(opt),
	}
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
		uu := new(URL)
		uu.Loc = u
		uu.Priority = 1.0
		c.queue.Push(uu)
		uu.Enqueue.Count++
		uu.Enqueue.Time = time.Now()
		if err := c.filter.sites.addURLs(uu); err != nil {
			return err
		}
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

	c.handler.In = c.fetcher.Out
	c.handler.Start()

	c.parser.In = c.handler.Out
	c.parser.Start()

	c.filter.In = c.parser.Out
	c.filter.Start(c.ctrl)

	go func() {
		for u := range c.filter.Out {
			c.queue.Push(u)
		}
	}()
}
