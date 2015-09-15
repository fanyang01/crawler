package crawler

import (
	"errors"
	"net/url"
)

type Crawler struct {
	ctrl    Controller
	option  *Option
	queue   *urlHeap
	handler *RespHandler
	pool    *Pool
	sites   map[string]*Site
}

type Option struct {
	PoolSize int
}

type Ctrl struct{}

func (c Ctrl) HandleResponse(resp *Response) {}
func (c Ctrl) Score(u *url.URL) float64      { return 0.5 }

var (
	DefaultOption = &Option{
		PoolSize: 32,
	}
	DefaultController = &Ctrl{}
)

func newCrawler(ctrl Controller, option *Option) *Crawler {
	if ctrl == nil {
		ctrl = DefaultController
	}
	if option == nil {
		option = DefaultOption
	}
	return &Crawler{
		queue:   newURLQueue(),
		handler: NewRespHandler(),
		pool:    NewPool(option.PoolSize),
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
		if site, err := NewSite(seed); err != nil {
			return err
		} else {
			c.sites[site.Root] = site
			// c.queue.Push(url)
		}
	}
	return nil
}

func (c *Crawler) Crawl() {
	c.pool.Work()
	urlChan := make(chan *URL)
	go func() {
		for {
			urlChan <- c.queue.Pop()
		}
	}()
	c.handler.Handle(c.pool.DoRequest(NewRequest(urlChan)))
	go func() {
		ch := ParseLink(c.handler.parser.ch)
		for doc := range ch {
			for _, u := range doc.SubURLs {
				uu := new(URL)
				uu.Loc = u
				c.queue.Push(uu)
			}
		}
	}()
}
