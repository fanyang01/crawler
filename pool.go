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
	Depth      int
	processing bool
	nextTime   time.Time
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

// client needs to acquire lock to perform Add and Get operation.
func (p *pool) Add(u *URL) {
	uu := u.clone()
	uu.Loc.Fragment = ""
	p.m[uu.Loc.String()] = uu
}

func (p *pool) Get(u url.URL) (*URL, bool) {
	u.Fragment = ""
	uu, ok := p.m[u.String()]
	if ok {
		uu = uu.clone()
	}
	return uu, ok
}
