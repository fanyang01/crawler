package crawler

import (
	"net/http"
	"net/url"
	"time"
)

type Response struct {
	*http.Response
	requestURL      *url.URL
	Ready           bool     // body closed?
	Locations       *url.URL // distinguish with method Location
	ContentLocation *url.URL
	ContentType     string
	Content         []byte
	Date            time.Time
	LastModified    time.Time
	Expires         time.Time
	Cacheable       bool
	Age             time.Duration
	MaxAge          time.Duration
}
