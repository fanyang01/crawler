package crawler

import (
	"net/url"
	"sync"
)

type cachePool struct {
	size    int64
	maxSize int64
	sync.RWMutex
	m map[string]*Response
}

func newCachePool(maxSize int64) *cachePool {
	return &cachePool{
		m:       make(map[string]*Response),
		maxSize: maxSize,
	}
}

func (cp *cachePool) Set(r *Response) {
	if !r.IsCacheable() || r.IsExpired() {
		return
	}
	us := r.NewURL.String()
	// TODO: maybe don't need copy?
	rr := newResponse()
	*rr = *r

	cp.Lock()
	defer cp.Unlock()

	for key := range cp.m {
		if cp.size+int64(len(r.Content)) <= cp.maxSize {
			break
		}
		cp.size -= int64(len(cp.m[key].Content))
		cp.m[key].free()
		delete(cp.m, key)
	}
	cp.m[us] = rr
	cp.size += int64(len(r.Content))
}

func (cp *cachePool) Get(u *url.URL) (r *Response, ok bool) {
	us := u.String()
	var rr *Response
	cp.RLock()
	rr, ok = cp.m[us]
	cp.RUnlock()
	if ok {
		r = newResponse()
		*r = *rr
	}
	return
}

func (cp *cachePool) Remove(u *url.URL) {
	us := u.String()
	cp.Lock()
	r := cp.m[us]
	delete(cp.m, us)
	if r != nil {
		cp.size -= r.length()
		r.free()
	}
	cp.Unlock()
}
