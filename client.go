package crawler

import (
	"fmt"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/gorilla/websocket"
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
	DefaultAjaxClient *AjaxClient
)

func init() {
	jar, err := cookiejar.New(nil)
	if err != nil {
		panic(err)
	}
	DefaultHTTPClient = &http.Client{
		Transport: DefaultHTTPTransport,
		Jar:       jar,
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
		"Method": req.Method,
		"Status": resp.Status,
		"URL":    req.URL.String(),
	}).Infoln()

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

// AjaxClient connects to an Electron instance through
// a websocket.
type AjaxClient struct {
	conn *websocket.Conn
}

func NewAjaxClient(wsAddr string) (*AjaxClient, error) {
	conn, _, err := websocket.DefaultDialer.Dial(wsAddr, http.Header{})
	if err != nil {
		return nil, err
	}
	return &AjaxClient{
		conn: conn,
	}, nil
}

type requestMsg struct {
	URL     string      `json:"url,omitempty"`
	Headers http.Header `json:"headers,omitempty"`
	Proxy   string      `json:"proxy,omitempty"`
	Cookies []struct {
		Name  string `json:"name,omitempty"`
		Value string `json:"value,omitempty"`
	} `json:"cookies,omitempty"`
}

func reqToMsg(req *Request) *requestMsg {
	m := &requestMsg{
		URL:     req.URL.String(),
		Headers: req.Header,
		Proxy:   req.Proxy.String(),
	}
	for _, cookie := range req.Cookies {
		m.Cookies = append(m.Cookies, struct {
			Name  string `json:"name,omitempty"`
			Value string `json:"value,omitempty"`
		}{Name: cookie.Name, Value: cookie.Value})
	}
	return m
}

type responseMsg struct {
	NewURL        string      `json:"newURL"`
	OriginalURL   string      `json:"originalURL"`
	RequestMethod string      `json:"requestMethod"`
	StatusCode    int         `json:"statusCode,omitempty"`
	Content       []byte      `json:"content,omitempty"`
	Headers       http.Header `json:"headers"`
	Cookies       []struct {
		Name  string `json:"name,omitempty"`
		Value string `json:"value,omitempty"`
	} `json:"cookies,omitempty"`
}

func msgToResp(msg *responseMsg) *Response {
	r := &http.Response{
		Status:     http.StatusText(msg.StatusCode),
		StatusCode: msg.StatusCode,
		Proto:      "HTTP/1.0",
		ProtoMajor: 1,
		ProtoMinor: 0,
		Header:     msg.Headers,
		Request: &http.Request{
			Method: msg.RequestMethod,
		},
	}
	if u, err := url.Parse(msg.OriginalURL); err == nil {
		r.Request.URL = u
	}
	if r.Header == nil {
		r.Header = http.Header{}
	}
	if r.Header.Get("Location") == "" {
		r.Header.Set("Location", msg.NewURL)
	}
	return &Response{
		Response:   r,
		Content:    msg.Content,
		BodyStatus: BodyStatusEOF,
	}
}

func (c *AjaxClient) Do(req *Request) (resp *Response, err error) {
	if err = c.conn.WriteJSON(reqToMsg(req)); err != nil {
		return
	}
	var rp responseMsg
	if err = c.conn.ReadJSON(&rp); err != nil {
		return
	}
	resp = msgToResp(&rp)
	resp.URL = req.URL
	resp.parseLocation()
	return
}
