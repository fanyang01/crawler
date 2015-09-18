package crawler

import (
	"net/url"
	"sync"
	"time"

	"github.com/fanyang01/crawler/sitemap"
)

type pool struct {
	sync.RWMutex
	m map[string]*URL
}

type URL struct {
	sitemap.URL
	Score   int64
	Visited struct {
		Count int
		Time  time.Time
	}
	Enqueue struct {
		Count int
		Time  time.Time
	}
	Future time.Time
}

func (u *URL) clone() *URL {
	uu := *u
	return &uu
}

func newURL(u url.URL) *URL {
	u.Fragment = ""
	uu := new(URL)
	uu.Loc = u
	return uu
}

func newPool() *pool {
	return &pool{
		m: make(map[string]*URL),
	}
}

func (p *pool) Add(u *URL) {
	uu := u.clone()
	uu.Loc.Fragment = ""
	p.Lock()
	p.m[uu.Loc.String()] = uu
	p.Unlock()
}

func (p *pool) Get(u url.URL) (*URL, bool) {
	u.Fragment = ""
	p.RLock()
	defer p.RUnlock()
	uu, ok := p.m[u.String()]
	if ok {
		uu = uu.clone()
	}
	return uu, ok
}
