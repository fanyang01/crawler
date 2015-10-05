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

type storeEntry struct {
	url URL
	sync.Mutex
}

type store struct {
	sync.RWMutex
	m map[url.URL]*storeEntry
}

func newURL(u url.URL) *URL {
	u.Fragment = ""
	uu := new(URL)
	uu.Loc = u
	return uu
}

func newPool() *store {
	return &store{
		m: make(map[url.URL]*storeEntry),
	}
}

// Get returns a  a locked entry.
// If key u.Loc is not present in map, it will create a new entry.
func (p *store) Get(u URL) *storeEntry {
	p.Lock()
	defer p.Unlock()
	u.Loc.Fragment = ""
	entry, ok := p.m[u.Loc]
	if ok {
		entry.Lock()
		return entry
	}

	entry = &storeEntry{
		url: u,
	}
	entry.Lock()
	p.m[u.Loc] = entry
	return entry
}
