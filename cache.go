package crawler

import (
	"net/url"
	"sync"
	"time"
)

// TODO: control the cache pool size
type cachePool struct {
	sync.RWMutex
	m map[string]*Response
}

func newCachePool() *cachePool {
	return &cachePool{
		m: make(map[string]*Response),
	}
}

func (cp *cachePool) Add(resp *Response) {
	if !resp.Cacheable {
		return
	}
	if resp.Expires.Before(time.Now()) {
		return
	}
	cp.Lock()
	cp.m[resp.Locations.String()] = resp
	cp.Unlock()
}

func (cp *cachePool) Get(URL string) (resp *Response, ok bool) {
	u, err := url.Parse(URL)
	if err != nil {
		return
	}
	cp.RLock()
	defer cp.RUnlock()
	resp, ok = cp.m[u.String()]
	if !ok || resp.Expires.Before(time.Now()) {
		return nil, false
	}
	return
}
