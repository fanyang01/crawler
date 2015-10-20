package crawler

import (
	"net/url"
	"sync"
	"time"
)

// StoreHandle is a handle for modifying metadata of a url.
type StoreHandle interface {
	// V provides a pointer for modifying internal data.
	// If data is stored in db rather than memory, this method
	// must retrieve and store it in memory.
	V() *URL
	// Unlock may need to update data associated with the handle
	// in addition to unlock the handle, for instance, writing it back to db.
	Unlock()
}

// URLStore stores all URLs.
type URLStore interface {
	Get(u url.URL) (URL, bool)
	Put(u URL)
	// Watch locks the entry located by u and returns a handle.
	Watch(u url.URL) StoreHandle
	// WatchP locks the entry(if not exist, create)
	WatchP(u URL) StoreHandle
}

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
	Done         bool
	nextTime     time.Time
}

type entry struct {
	url URL
	sync.Mutex
}

func (entry *entry) V() *URL {
	return &entry.url
}

type store struct {
	sync.RWMutex
	m map[url.URL]*entry
}

func newURL(u url.URL) *URL {
	u.Fragment = ""
	return &URL{
		Loc: u,
	}
}

func newMemStore() *store {
	return &store{
		m: make(map[url.URL]*entry),
	}
}

func (p *store) Watch(u url.URL) (h StoreHandle) {
	p.RLock()
	defer p.RUnlock()
	entry, ok := p.m[u]
	if !ok {
		return
	}
	entry.Lock()
	h = entry
	return
}

func (p *store) WatchP(u URL) StoreHandle {
	p.Lock()
	defer p.Unlock()
	u.Loc.Fragment = ""
	ent, ok := p.m[u.Loc]
	if ok {
		ent.Lock()
		return ent
	}

	ent = &entry{url: u}
	ent.Lock()
	p.m[u.Loc] = ent
	return ent
}

func (p *store) Put(u URL) {
	u.Loc.Fragment = ""
	p.Lock()
	p.m[u.Loc] = &entry{url: u}
	p.Unlock()
}

func (p *store) Get(u url.URL) (uu URL, ok bool) {
	u.Fragment = ""
	p.RLock()
	entry, present := p.m[u]
	if present {
		entry.Lock()
		uu, ok = entry.url, true
		entry.Unlock()
	}
	p.RUnlock()
	return
}
