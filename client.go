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
	DefaultClient = &StdClient{DefaultHTTPClient}
}

// StdClient is used for static pages.
type StdClient struct {
	*http.Client
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
	defer func() {
		if err != nil {
			resp.free()
			resp = nil
		}
	}()

	if c.Client == nil {
		c.Client = DefaultHTTPClient
	}
	resp = newResponse()
	resp.RequestURL = req.URL
	resp.Response, err = c.Client.Do(req.Request)
	if err != nil {
		return
	}
	resp.NewURL = resp.Request.URL

	logrus.WithFields(logrus.Fields{
		"URL": req.URL.String(),
	}).Infoln(req.Method, resp.Status)

	// Only status code 2xx is ok
	switch {
	case 200 <= resp.StatusCode && resp.StatusCode < 300:
		// Do nothing
	case resp.StatusCode == 304:
		// Do nothing
	default:
		err = ResponseStatusError(resp.StatusCode)
		return
	}
	return
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
		BodyStatus: RespStatusReady,
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
	resp.RequestURL = req.URL
	resp.parseLocation()
	return
}
