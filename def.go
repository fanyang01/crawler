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
