package crawler

import (
	"net/url"
	"sync"
	"time"
)

// URL contains metadata of a url in crawler.
type URL struct {
	Loc     url.URL
	Score   int64
	Freq    time.Duration
	Visited struct {
		Count int
		Time  time.Time
	}
	Depth        int
	LastModified time.Time
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
	PutIfNonExist(u *URL) bool
	UpdateVisit(u *url.URL, at, lastmod time.Time)
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

func (p *store) PutIfNonExist(u *URL) bool {
	p.Lock()
	defer p.Unlock()
	if _, ok := p.m[u.Loc]; ok {
		return false
	}
	p.m[u.Loc] = u.clone()
	return true
}

func (p *store) UpdateVisit(u *url.URL, at, lastmod time.Time) {
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
	uu.Visited.Time = at
	uu.LastModified = lastmod
}
