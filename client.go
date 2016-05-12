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

// Client defines how requests are made.
type Client interface {
	Do(*Request) (*Response, error)
}

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
	// DefaultClient is the default client used by crawler to fetch static
	// resources.
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
	DefaultClient = &StdClient{client: DefaultHTTPClient}
}

// StdClient is intended to be used for fetching static resources.
type StdClient struct {
	client *http.Client
	cache  *cache.Pool
}

// NewStdClient returns a new standard client that uses the provided HTTP
// client to do HTTP requests. cacheSize specifies the maxmium size(in
// bytes) of the HTTP cache pool. If cacheSize <= 0, HTTP caching will be
// disabled.
func NewStdClient(client *http.Client, cacheSize int) *StdClient {
	if client == nil {
		client = DefaultHTTPClient
	}
	c := &StdClient{client: client}
	if cacheSize > 0 {
		c.cache = cache.NewPool(cacheSize)
	}
	return c
}

// ResponseStatusError represents unexpected response status.
type ResponseStatusError int

func (e ResponseStatusError) Error() string {
	if e == 0 {
		return ""
	}
	return fmt.Sprintf("unexpected response status: %d %s", int(e), http.StatusText(int(e)))
}

// Do implements the Client interface.
func (c *StdClient) Do(req *Request) (r *Response, err error) {
	defer func() {
		if err != nil && r != nil {
			r.free()
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
	if req.Method == "GET" && c.cache != nil {
		if hr, body, cc, ok = c.cache.Get(req.URL); ok {
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

	if hr, err = c.client.Do(req.Request); err != nil {
		return nil, RetryableError{Err: err}
	}
	now = time.Now()

	// Only status code 2xx is OK.
	switch {
	case 200 <= hr.StatusCode && hr.StatusCode < 300:
	// 5xx and 4xx but 404 are retryable.
	case hr.StatusCode >= 500:
		fallthrough
	case hr.StatusCode >= 400 && hr.StatusCode != 404:
		hr.Body.Close()
		err = RetryableError{
			Err: ResponseStatusError(hr.StatusCode),
		}
		return
	default:
		hr.Body.Close()
		err = ResponseStatusError(hr.StatusCode)
		return
	}
	if c.cache != nil {
		cc = cache.Parse(hr, now)
	}

INIT:
	r = NewResponse()
	r.init(req.URL, hr, now, cc)
	if c.cache != nil && cc != nil && cc.IsCacheable() {
		if !modified { // Just update cached header
			if ok := c.cache.Update(req.URL, r.CacheControl, r.Header); ok {
				return
			}
		}
		r.Body = c.cache.NewReader(r.NewURL, r.CacheControl, r.Response, r.Body)
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

	if rr, err = c.client.Do(req); err != nil {
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
			c.cache.Remove(u)
		}
		return
	// 5xx and 4xx but 404 are retryable.
	case rr.StatusCode >= 500:
		fallthrough
	case rr.StatusCode >= 400 && rr.StatusCode != 404:
		rr.Body.Close()
		err = RetryableError{
			Err: ResponseStatusError(rr.StatusCode),
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
