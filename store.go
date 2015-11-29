package crawler

import (
	"net/url"
	"sync"
	"time"
)

const (
	// Status of a URL.
	URLprocessing = iota
	URLfinished
	URLerror
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
	ErrCount int
	Status   int
}

func (u *URL) clone() *URL {
	uu := *u
	return &uu
}

// Store stores all URLs.
type Store interface {
	Exist(u *url.URL) bool
	Get(u *url.URL) (*URL, bool)
	GetDepth(u *url.URL) int
	PutNX(u *URL) bool
	UpdateVisited(u *url.URL, at, lastmod time.Time)
	SetStatus(u *url.URL, status int)
	IncErrCount(u *url.URL) int

	IncNTime()
	IncNError()
	AllFinished() bool
}

type memStore struct {
	sync.RWMutex
	m map[url.URL]*URL

	URLs     int32
	Finished int32
	Ntimes   int32
	Errors   int32
}

func newMemStore() *memStore {
	return &memStore{
		m: make(map[url.URL]*URL),
	}
}

func (p *memStore) Exist(u *url.URL) bool {
	p.RLock()
	defer p.RUnlock()
	_, ok := p.m[*u]
	return ok
}

func (p *memStore) Get(u *url.URL) (uu *URL, ok bool) {
	p.RLock()
	entry, present := p.m[*u]
	if present {
		uu, ok = entry.clone(), true
	}
	p.RUnlock()
	return
}

func (p *memStore) GetDepth(u *url.URL) int {
	p.RLock()
	defer p.RUnlock()
	if uu, ok := p.m[*u]; ok {
		return uu.Depth
	}
	return 0
}

func (p *memStore) PutNX(u *URL) bool {
	p.Lock()
	defer p.Unlock()
	if _, ok := p.m[u.Loc]; ok {
		return false
	}
	p.m[u.Loc] = u.clone()
	p.URLs++
	return true
}

func (p *memStore) UpdateVisited(u *url.URL, at, lastmod time.Time) {
	p.Lock()
	defer p.Unlock()
	uu, ok := p.m[*u]
	if !ok {
		return
	}
	uu.Visited.Count++
	uu.Visited.LastTime = at
	uu.LastMod = lastmod
	uu.ErrCount = 0
}

func (p *memStore) SetStatus(u *url.URL, status int) {
	p.Lock()
	defer p.Unlock()

	uu, ok := p.m[*u]
	if !ok {
		return
	}
	uu.Status = status
	switch status {
	case URLfinished, URLerror:
		p.Finished++
	}
}

func (p *memStore) IncErrCount(u *url.URL) int {
	p.Lock()
	defer p.Unlock()
	uu, ok := p.m[*u]
	if !ok {
		return 0
	}
	uu.ErrCount++
	return uu.ErrCount
}

func (s *memStore) IncNTime() {
	s.Lock()
	s.Ntimes++
	s.Unlock()
}

func (s *memStore) IncNError() {
	s.Lock()
	s.Errors++
	s.Unlock()
}

func (s *memStore) AllFinished() bool {
	s.RLock()
	defer s.RUnlock()
	return s.Finished >= s.URLs
}
