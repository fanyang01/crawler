// Package crawler provides a flexible web crawler.
package crawler

import (
	"net/http"
	"net/url"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// RequestType defines the type of a request.
type RequestType int

const (
	// Static content that can be reached by simple HTTP method.
	ReqStatic RequestType = iota
	// Dynamic pages that require a browser to complete rendering process.
	ReqDynamic
)

// Client defines how requests
type Client interface {
	Do(*Request) (*Response, error)
}

// Request is a HTTP request to be made.
type Request struct {
	*http.Request
	Proxy   *url.URL
	Cookies []*http.Cookie
	Type    RequestType

	// Client is the client used to do this request. If nil,
	// DefaultClient or DefaultAjaxClient is used, depending on Type.
	Client Client
}

// Response contains a http response and some metadata.
// Note the body of response may be read or not, depending on
// the type of content and the size of content. Call ReadBody to
// safely read and close the body. Optionally, you can access Body
// directly but do NOT close it.
type Response struct {
	*http.Response
	// RequestURL is the original url used to do request that finally
	// produces this response.
	RequestURL      *url.URL
	Ready           bool     // body read and closed?
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
	// content will be parsed into document only if neccessary.
	document *goquery.Document
}

// Controller controls the working process of crawler.
type Controller interface {
	// Prepare sets options(client, headers, ...) for a http request
	Prepare(*Request)

	// Schedule gives a score between 0 and 1024 for a URL, Higher score means
	// higher priority in queue.  Schedule also specifies the next time that
	// this URL should be crawled at. If this URL is expected to be not crawled
	// any more, return true for done.
	Schedule(u *URL) (score int, at time.Time, done bool)

	// Accept determines whether a URL should be processed. It acts as a
	// blacklist that preventing some unneccesary URLs to be processed.
	Accept(anchor *Anchor) bool

	// Handle handles a response. If the content type of
	// response is text/html, the body of the response is prefetched. Some
	// utils are provided to handle html document.
	Handle(resp *Response) bool
}
