package crawler

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"time"

	"github.com/fanyang01/crawler/cache"
)

var (
	DefaultHTTPTransport = &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		Dial: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).Dial,
		TLSHandshakeTimeout: 5 * time.Second,
	}
	// DefaultHTTPClient uses DefaultHTTPTransport to make HTTP request,
	// and enables the cookie jar.
	DefaultHTTPClient *http.Client
	// DefaultClient is the default client used to fetch static pages.
	DefaultClient *StdClient
)

func init() {
	jar, err := cookiejar.New(nil)
	if err != nil {
		panic(err)
	}
	DefaultHTTPClient = &http.Client{
		Transport: DefaultHTTPTransport,
		Jar:       jar,
		Timeout:   10 * time.Second,
	}
	DefaultClient = &StdClient{Client: DefaultHTTPClient}
}

// StdClient is used for static pages.
type StdClient struct {
	*http.Client
	Cache *cache.Pool
}

// ResponseStatusError represents unexpected response status.
type ResponseStatusError int

func (e ResponseStatusError) Error() string {
	if e == 0 {
		return ""
	}
	return fmt.Sprintf("unexpected response status: %d %s", int(e), http.StatusText(int(e)))
}

// Do implements Client.
func (c *StdClient) Do(req *Request) (r *Response, err error) {
	defer func() {
		if err != nil && r != nil {
			r.Free()
		}
	}()

	var (
		hr       *http.Response
		cc       *cache.Control
		body     []byte
		now      time.Time
		ok       bool
		modified bool = true
	)
	if req.Method == "GET" && c.Cache != nil {
		if hr, body, cc, ok = c.Cache.Get(req.URL); ok {
			if cc.NeedValidate() {
				if hr, cc, modified, err = c.revalidate(
					req.URL, hr, body, cc,
				); err != nil {
					return
				}
			} else {
				modified = false
				hr.Body = ioutil.NopCloser(bytes.NewReader(body))
			}
			now = time.Now()
			goto INIT
		}
	}

	if hr, err = c.Client.Do(req.Request); err != nil {
		return
	}
	now = time.Now()

	// Only status code 2xx is OK.
	switch {
	case 200 <= hr.StatusCode && hr.StatusCode < 300:
	default:
		err = ResponseStatusError(hr.StatusCode)
		hr.Body.Close()
		return
	}
	if c.Cache != nil {
		cc = cache.Parse(hr, now)
	}

INIT:
	r = NewResponse()
	r.init(req.URL, hr, now, cc)
	if c.Cache != nil && cc != nil && cc.IsCacheable() {
		if !modified { // Just update cached header
			if ok := c.Cache.Update(req.URL, r.CacheControl, r.Header); ok {
				return
			}
		}
		r.Body = c.Cache.NewReader(r.NewURL, r.CacheControl, r.Response, r.Body)
	}
	return
}

func (c *StdClient) revalidate(
	u *url.URL, r *http.Response, body []byte, cc *cache.Control,
) (
	rr *http.Response, rcc *cache.Control, modified bool, err error,
) {
	modified = true

	req, _ := http.NewRequest("GET", u.String(), nil)
	if cc.ETag != "" {
		req.Header.Add("If-None-Match", cc.ETag)
	}
	var t time.Time
	if t = cc.LastModified; t.IsZero() {
		t = cc.Date
	}
	req.Header.Add("If-Modified-Since", t.Format(http.TimeFormat))

	if rr, err = c.Client.Do(req); err != nil {
		return
	}

	switch {
	case rr.StatusCode == 304:
		rr.Body.Close()
		rr = cache.Construct(r, rr, body)
		if rr.Request.URL.String() == u.String() {
			modified = false
		}
		fallthrough
	case 200 <= rr.StatusCode && rr.StatusCode < 300:
		rcc = cache.Parse(rr, time.Now())
		if rcc == nil || !rcc.IsCacheable() {
			c.Cache.Remove(u)
		}
		return
	default:
		rr.Body.Close()
		err = ResponseStatusError(rr.StatusCode)
		return
	}
}

func (r *Response) init(u *url.URL, hr *http.Response,
	t time.Time, cc *cache.Control) *Response {

	r.URL = u
	r.NewURL = hr.Request.URL
	r.Timestamp = t
	r.Response = hr
	r.CacheControl = cc
	r.InitBody(hr.Body)
	return r
}
