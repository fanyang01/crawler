package crawler

import (
	"net/url"
	"sync"
	"time"
)

// URL contains metadata of a url in crawler.
type URL struct {
	Loc     url.URL
	Freq    time.Duration
	Depth   int
	LastMod time.Time
	Visited struct {
		Count    int
		LastTime time.Time
	}
}

func (u *URL) clone() *URL {
	uu := *u
	return &uu
}

// URLStore stores all URLs.
type URLStore interface {
	Exist(u *url.URL) bool
	Get(u *url.URL) (*URL, bool)
	GetDepth(u *url.URL) int
	PutNX(u *URL) bool
	VisitAt(u *url.URL, at, lastmod time.Time)
}

type store struct {
	sync.RWMutex
	m map[url.URL]*URL
}

func newMemStore() *store {
	return &store{
		m: make(map[url.URL]*URL),
	}
}

func (p *store) Exist(u *url.URL) bool {
	p.RLock()
	defer p.RUnlock()
	_, ok := p.m[*u]
	return ok
}

func (p *store) Get(u *url.URL) (uu *URL, ok bool) {
	p.RLock()
	entry, present := p.m[*u]
	if present {
		uu, ok = entry.clone(), true
	}
	p.RUnlock()
	return
}

func (p *store) GetDepth(u *url.URL) int {
	p.RLock()
	defer p.RUnlock()
	if uu, ok := p.m[*u]; ok {
		return uu.Depth
	}
	return 0
}

func (p *store) PutNX(u *URL) bool {
	p.Lock()
	defer p.Unlock()
	if _, ok := p.m[u.Loc]; ok {
		return false
	}
	p.m[u.Loc] = u.clone()
	return true
}

func (p *store) VisitAt(u *url.URL, at, lastmod time.Time) {
	p.Lock()
	defer p.Unlock()
	uu, ok := p.m[*u]
	if !ok {
		uu = &URL{
			Loc: *u,
		}
		p.m[*u] = uu
	}
	uu.Visited.Count++
	uu.Visited.LastTime = at
	uu.LastMod = lastmod
}
