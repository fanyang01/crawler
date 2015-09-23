package crawler

import (
	"net/url"
	"sync"
	"time"

	"github.com/fanyang01/crawler/sitemap"
)

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

type poolEntry struct {
	url URL
	sync.Mutex
}

type pool struct {
	sync.RWMutex
	m map[url.URL]*poolEntry
}

func newURL(u url.URL) *URL {
	u.Fragment = ""
	uu := new(URL)
	uu.Loc = u
	return uu
}

func newPool() *pool {
	return &pool{
		m: make(map[url.URL]*poolEntry),
	}
}

// Get returns a  a locked entry.
// If key u.Loc is not present in map, it will create a new entry.
func (p *pool) Get(u URL) *poolEntry {
	p.Lock()
	defer p.Unlock()
	u.Loc.Fragment = ""
	entry, ok := p.m[u.Loc]
	if ok {
		entry.Lock()
		return entry
	}

	entry = &poolEntry{
		url: u,
	}
	entry.Lock()
	p.m[u.Loc] = entry
	return entry
}
