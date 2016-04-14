package crawler

import (
	"net/url"
	"sync"
	"time"
)

const (
	CacheDisallow = iota
	CacheNeedValidate
	CacheNormal
)

type CacheControl struct {
	CacheType    int
	Date         time.Time
	Timestamp    time.Time
	Age          time.Duration
	MaxAge       time.Duration
	ETag         string
	LastModified time.Time
}

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
	us := r.URL.String()
	// TODO: validate that it's safe to save the pointer
	// rr := newResponse()
	// *rr = *r

	cp.Lock()
	defer cp.Unlock()

	for key := range cp.m {
		if cp.size+r.length() <= cp.maxSize {
			break
		}
		cp.size -= cp.m[key].length()
		cp.m[key].free()
		delete(cp.m, key)
	}
	cp.m[us] = r
	cp.size += r.length()
}

func (cp *cachePool) Get(u *url.URL) (r *Response, ok bool) {
	us := u.String()
	cp.RLock()
	r, ok = cp.m[us]
	cp.RUnlock()
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
