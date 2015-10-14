package crawler

import (
	"net/url"
	"sync"
	"time"
)

type CachePool struct {
	size    int64
	maxSize int64
	sync.RWMutex
	m map[string]*Response
}

func NewCachePool(maxSize int64) *CachePool {
	return &CachePool{
		m:       make(map[string]*Response),
		maxSize: maxSize,
	}
}

func (cp *CachePool) Add(r *Response) {
	cp.Lock()
	defer cp.Unlock()
	for key := range cp.m {
		if cp.size+int64(len(r.Content)) <= cp.maxSize {
			break
		}
		cp.size -= int64(len(cp.m[key].Content))
		cp.m[key] = nil
		delete(cp.m, key)
	}
	resp := *r
	if !resp.Cacheable {
		return
	}
	if resp.Expires.Before(time.Now()) {
		return
	}
	u0 := resp.Locations.String()
	u1 := resp.RequestURL.String()
	cp.m[u0] = &resp
	if u1 != u0 {
		cp.m[u1] = &resp
	}
	cp.size += int64(len(r.Content))
}

func (cp *CachePool) Get(URL string) (resp *Response, ok bool) {
	u, err := url.Parse(URL)
	if err != nil {
		return
	}
	cp.Lock()
	defer cp.Unlock()
	var r *Response
	if r, ok = cp.m[u.String()]; !ok {
		return
	}
	if r.Expires.Before(time.Now()) {
		delete(cp.m, u.String())
		return nil, false
	}
	rr := *r
	// NOTE: it's IMPORTANT to update response's time
	rr.Date = time.Now()
	return &rr, true
}
