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
	u0 := resp.Locations.String()
	u1 := resp.requestURL.String()
	cp.Lock()
	cp.m[u0] = resp
	if u1 != u0 {
		cp.m[u1] = resp
	}
	cp.Unlock()
}

func (cp *cachePool) Get(URL string) (resp *Response, ok bool) {
	u, err := url.Parse(URL)
	if err != nil {
		return
	}
	cp.Lock()
	defer cp.Unlock()
	resp, ok = cp.m[u.String()]
	if !ok {
		return
	}
	if resp.Expires.Before(time.Now()) {
		delete(cp.m, u.String())
		return nil, false
	}
	// NOTE: it's IMPORTANT to update response's time
	resp.Date = time.Now()
	return
}
