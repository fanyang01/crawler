// Package cache implements parts of HTTP caching protocol.
package cache

import (
	"bytes"
	"io"
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
	default:
		return nil
	}
	cc.Timestamp = rt
	cc.Date = rt
	if t := r.Header.Get("Date"); t != "" {
		if cc.Date, err = time.Parse(http.TimeFormat, t); err == nil {
			cc.Date = rt
		}
	}
	if s := r.Header.Get("Cache-Control"); s != "" {
		kv = parseCacheControl(s)
		var sec int64 = -1
		if v, ok := kv["max-age"]; ok {
			if i, err := strconv.ParseInt(v, 0, 32); err == nil {
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
	// Cachable
	cc.CacheType = CacheNormal
	switch {
	case exist("no-store"):
		return nil
	case exist("no-cache"):
		maxAge = 0
		cc.CacheType = CacheNeedValidate
	case exist("must-revalidate"):
		maxAge = 0
		cc.CacheType = CacheNormal // TODO: special type?
	case maxAge < 0:
		return nil
	}
	cc.MaxAge = maxAge

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
		}
		kv[name] = val
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

func NewPool(max int) *Pool {
	return &Pool{
		m:   make(map[string]*entry),
		max: max,
	}
}

func (p *Pool) Set(u *url.URL, cc *Control, r *http.Response, b []byte) {
	if cc == nil || !cc.IsCacheable() {
		return
	}
	us := u.String()

	p.Lock()
	defer p.Unlock()

	if e := p.m[us]; e != nil {
		p.size -= len(e.body)
		delete(p.m, us)
	}
	for k, e := range p.m {
		if p.size+len(b) <= p.max {
			break
		}
		p.size -= len(e.body)
		delete(p.m, k)
	}
	p.m[us] = &entry{
		r:    r,
		ctrl: cc,
		body: b,
	}
	p.size += len(b)
}

func (p *Pool) Update(u *url.URL, cc *Control, h http.Header) bool {
	us := u.String()
	p.Lock()
	defer p.Unlock()

	e := p.m[us]
	if e == nil {
		return false
	}
	// rfc2616 13.12 Cache Replacement
	if cc.Date.Before(e.ctrl.Date) {
		return true
	}
	if cc == nil || !cc.IsCacheable() {
		p.remove(us)
		return true
	}
	e.ctrl = cc
	e.r.Header = h
	return true
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
	p.remove(us)
	p.Unlock()
}

func (p *Pool) remove(us string) {
	r := p.m[us]
	delete(p.m, us)
	if r != nil {
		p.size -= len(r.body)
	}
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

type cacheReader struct {
	url      *url.URL
	response *http.Response
	cc       *Control
	buf      *bytes.Buffer
	pool     *Pool
	err      error
	status   int
	tee      io.Reader
}

func (p *Pool) NewReader(u *url.URL, cc *Control,
	r *http.Response, reader io.Reader) io.Reader {

	buf := new(bytes.Buffer)
	return &cacheReader{
		url:      u,
		cc:       cc,
		response: r,
		buf:      buf,
		pool:     p,
		tee:      io.TeeReader(reader, buf),
	}
}

const (
	statusReading = iota
	statusEOF
	statusError
)

func (c *cacheReader) Read(p []byte) (n int, err error) {
	switch c.status {
	case statusReading:
		n, err = c.tee.Read(p)
		if err != nil {
			if err == io.EOF {
				c.status = statusEOF
				c.pool.Set(c.url, c.cc, c.response, c.buf.Bytes())
			} else {
				c.status = statusError
			}
			c.err = err
		}
	case statusEOF, statusError:
		return 0, c.err
	}
	return
}
