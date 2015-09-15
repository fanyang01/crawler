package crawler

import (
	"errors"
	"net/url"
)

type Crawler struct {
	ctrl    Controller
	queue   *urlHeap
	handler *RespHandler
	pool    *Pool
	sites   map[string]*Site
}

var defaultPoolSize = 8

func newCrawler() *Crawler {
	return &Crawler{
		queue:   newURLQueue(),
		handler: NewRespHandler(),
		pool:    NewPool(defaultPoolSize),
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
