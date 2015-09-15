package crawler

import (
	"errors"
	"log"
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

func (c Ctrl) HandleResponse(resp *Response) { log.Println(resp.Locations) }
func (c Ctrl) Score(u *url.URL) float64      { return 0.5 }

var (
	DefaultOption = &Option{
		PoolSize: 32,
	}
	DefaultController = &Ctrl{}
)

func NewCrawler(ctrl Controller, option *Option) *Crawler {
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
		sites:   make(map[string]*Site),
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
	c.pool.Work()
	log.Println("here 1")
	urlChan := make(chan *URL, 64)
	go func() {
		for {
			urlChan <- c.queue.Pop()
		}
	}()
	log.Println("here 2")
	go c.handler.Handle(c.pool.DoRequest(NewRequest(urlChan)))
	go func() {
		ch := ParseLink(c.handler.parser.ch)
		for doc := range ch {
			for _, u := range doc.SubURLs {
				log.Println(u)
				uu := new(URL)
				uu.Loc = u
				c.queue.Push(uu)
			}
		}
	}()
	log.Println("here 3")
}
