package cache

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	CacheDisallow = iota
	CacheNeedValidate
	CacheNormal
)

type Control struct {
	CacheType    int
	Date         time.Time
	Timestamp    time.Time
	Age          time.Duration
	MaxAge       time.Duration
	ETag         string
	LastModified time.Time
}

func (cc *Control) IsExpired() bool {
	age := computeAge(cc.Date, cc.Timestamp, cc.Age)
	if age > cc.MaxAge {
		return true
	}
	return false
}

// Use a simplified calculation of rfc2616-sec13.
func computeAge(date, resp time.Time, age time.Duration) time.Duration {
	apparent := max64(0, resp.Sub(date))
	recv := max64(apparent, age)
	// assume delay = 0
	// initial := recv + delay
	resident := time.Now().Sub(resp)
	return recv + resident
}

func max64(x, y time.Duration) time.Duration {
	if x > y {
		return x
	}
	return y
}

// https://www.w3.org/Protocols/rfc2616/rfc2616-sec13.html
func Parse(r *http.Response, rt time.Time) *Control {
	var (
		cc     Control
		err    error
		maxAge = time.Duration(-1)
		kv     = map[string]string{}
	)

	switch r.StatusCode {
	case 200, 203, 206, 300, 301:
		// Do nothing
	default:
		return nil
	}
	cc.Timestamp = rt

	if t := r.Header.Get("Date"); t != "" {
		if cc.Date, err = time.Parse(http.TimeFormat, t); err != nil {
			cc.Date = rt
		}
	}
	if s := r.Header.Get("Cache-Control"); s != "" {
		kv = parseCacheControl(s)
		var sec int64 = -1
		if v, ok := kv["max-age"]; ok {
			if i, err := strconv.ParseInt(v, 0, 32); err != nil {
				sec = i
			}
		}
		if v, ok := kv["s-maxage"]; ok {
			if i, err := strconv.ParseInt(v, 0, 32); err == nil && i > sec {
				sec = i
			}
		}
		if sec >= 0 {
			maxAge = time.Duration(sec) * time.Second
		} else {
			if t := r.Header.Get("Expires"); t != "" {
				expire, err := time.Parse(http.TimeFormat, t)
				if err == nil && !cc.Date.IsZero() {
					maxAge = expire.Sub(cc.Date)
				}
			}
		}
	}
	exist := func(directive string) bool {
		_, ok := kv[directive]
		return ok
	}
	if maxAge < 0 || exist("no-store") {
		return nil
	}
	// Cachable
	cc.CacheType = CacheNormal
	cc.MaxAge = maxAge
	switch {
	case exist("no-cache"):
		cc.CacheType = CacheNeedValidate
	case exist("must-revalidate"):
		cc.CacheType = CacheNormal // TODO: special type?
	}

	var age time.Duration
	if a := r.Header.Get("Age"); a != "" {
		if seconds, err := strconv.ParseInt(a, 0, 32); err == nil {
			age = time.Duration(seconds) * time.Second
		}
	}
	cc.Age = computeAge(cc.Date, rt, age)

	cc.ETag = r.Header.Get("ETag")
	if t := r.Header.Get("Last-Modified"); t != "" {
		cc.LastModified, _ = time.Parse(http.TimeFormat, t)
	}
	return &cc
}

func parseCacheControl(s string) (kv map[string]string) {
	kv = make(map[string]string)
	parts := strings.Split(strings.TrimSpace(s), ",")
	if len(parts) == 1 && parts[0] == "" {
		return
	}
	for i := 0; i < len(parts); i++ {
		parts[i] = strings.TrimSpace(parts[i])
		if len(parts[i]) == 0 {
			continue
		}
		name, val := parts[i], ""
		if j := strings.Index(name, "="); j >= 0 {
			val = strings.TrimLeft(name[j+1:], " \t\r\n\f")
			name = strings.TrimRight(name[:j], " \t\r\n\f")
			if len(val) > 0 {
				kv[name] = val
			}
			continue
		}
		kv[name] = ""
	}
	return
}

func (cc *Control) IsCacheable() bool {
	switch cc.CacheType {
	case CacheNeedValidate, CacheNormal:
		return true
	}
	return false
}

func (cc *Control) NeedValidate() bool {
	return cc.CacheType == CacheNeedValidate ||
		(cc.CacheType == CacheNormal && cc.IsExpired())
}

type entry struct {
	r    *http.Response
	ctrl *Control
	body []byte
}

type Pool struct {
	size int
	max  int
	sync.RWMutex
	m map[string]*entry
}

func newCachePool(max int) *Pool {
	return &Pool{
		m:   make(map[string]*entry),
		max: max,
	}
}

func (p *Pool) Set(u *url.URL, cc *Control, r *http.Response, b []byte) {
	if cc == nil || !cc.IsCacheable() || cc.IsExpired() {
		return
	}
	us := u.String()

	p.Lock()
	defer p.Unlock()

	for key := range p.m {
		if p.size+len(b) <= p.max {
			break
		}
		p.size -= len(p.m[key].body)
		delete(p.m, key)
	}
	p.m[us] = &entry{
		r:    r,
		ctrl: cc,
		body: b,
	}
	p.size += len(b)
}

func (p *Pool) Get(u *url.URL) (r *http.Response, b []byte, cc *Control, ok bool) {
	us := u.String()
	p.RLock()
	e, ok := p.m[us]
	p.RUnlock()
	if ok {
		r, b, cc = e.r, e.body, e.ctrl
	}
	return
}

func (p *Pool) Remove(u *url.URL) {
	us := u.String()
	p.Lock()
	r := p.m[us]
	delete(p.m, us)
	if r != nil {
		p.size -= len(r.body)
	}
	p.Unlock()
}

func Construct(r, newr *http.Response, b []byte) *http.Response {
	// TODO: only copy end-to-end headers
	for k, vv := range newr.Header {
		r.Header.Del(k)
		for _, v := range vv {
			r.Header.Add(k, v)
		}
	}
	r.Body = ioutil.NopCloser(bytes.NewReader(b))
	return r
}
