package crawler

import (
	"fmt"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"time"

	"github.com/Sirupsen/logrus"
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
	// DefaultAjaxClient is the default client used to fetch dynamic pages.
	DefaultAjaxClient *ElectronWebsocket
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
	Cache *cachePool
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
func (c *StdClient) Do(req *Request) (resp *Response, err error) {
	var rr *Response
	if req.Method == "GET" && c.Cache != nil {
		var ok bool
		if rr, ok = c.Cache.Get(req.URL); ok {
			switch rr.CacheType {
			case CacheNormal:
				if !rr.IsExpired() {
					return rr, nil
				}
				fallthrough
			case CacheNeedValidate:
				return c.revalidate(rr)
			}
		}
	}

	resp = newResponse()
	defer func() {
		if err != nil {
			resp.free()
			resp = nil
		}
	}()

	var rp *http.Response
	if rp, err = c.Client.Do(req.Request); err != nil {
		return
	}
	switch {
	case 200 <= rp.StatusCode && rp.StatusCode < 300:
		// Only status code 2xx is ok
	default:
		err = ResponseStatusError(rp.StatusCode)
		rp.Body.Close()
		return
	}
	resp.init(req.URL, rp, time.Now())

	// Date is used by store
	resp.parseCache()
	if c.Cache != nil {
		c.Cache.Set(resp)
	}

	logrus.WithFields(logrus.Fields{
		"func": "StdClient.Do",
		"url":  req.URL.String(),
	}).Infoln(req.Method + " " + resp.Status)

	return
}

func (c *StdClient) revalidate(r *Response) (resp *Response, err error) {
	req, _ := http.NewRequest("GET", r.URL.String(), nil)
	if r.ETag != "" {
		req.Header.Add("If-None-Match", r.ETag)
	}
	var lm time.Time
	if lm = r.LastModified; lm.IsZero() {
		lm = r.Timestamp
	}
	req.Header.Add("If-Modified-Since", lm.Format(http.TimeFormat))

	var rp *http.Response
	if rp, err = c.Client.Do(req); err != nil {
		return
	}
	now := time.Now()
	addToCache := func(r *Response) {
		if r.parseCache(); r.IsCacheable() && !r.IsExpired() {
			c.Cache.Set(r)
		} else {
			c.Cache.Remove(r.URL)
		}
	}

	switch {
	case rp.StatusCode == 304:
		// TODO: only copy end-to-end headers
		for k, vv := range rp.Header {
			r.Header.Del(k)
			for _, v := range vv {
				r.Header.Set(k, v)
			}
		}
		r.Timestamp = now
		addToCache(r)
	case 200 <= rp.StatusCode && rp.StatusCode < 300:
		r.free()
		r = newResponse()
		r.init(req.URL, rp, now)
		addToCache(r)
	default:
		rp.Body.Close()
		err = ResponseStatusError(resp.StatusCode)
		return
	}
	return r, nil
}

func (resp *Response) init(u *url.URL, rp *http.Response, stamp time.Time) {
	resp.URL = u
	resp.NewURL = rp.Request.URL
	resp.Timestamp = stamp
	resp.Response = rp
	resp.initBody()
}
