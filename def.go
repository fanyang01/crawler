package crawler

import (
	"net/http"
	"net/url"
	"time"
)

type Doc struct {
	URL
	SecondURL   *url.URL
	SubURLs     []*url.URL
	Content     []byte
	ContentType string
	Time        time.Time
	Expires     time.Time
}

type Request struct {
	method, url string
	body        []byte
	client      *http.Client
	config      func(*http.Request)
}

type Response struct {
	*http.Response
	closed          bool     // body closed?
	Locations       *url.URL // distinguish with method Location
	ContentLocation *url.URL
	ContentType     string
	Content         []byte
	Time            time.Time
	LastModified    time.Time
	Expires         time.Time
	Cacheable       bool
	Age             time.Duration
	MaxAge          time.Duration
}

type Pool struct {
	size    int
	workers []Worker
	free    chan *Worker
	client  *http.Client
}

type Worker struct {
	req  chan *Request
	resp chan *Response
	err  chan error
	pool *Pool
}
