// Package crawler provides a flexible web crawler.
package crawler

import (
	"net/http"
	"net/url"
	"time"
)

const (
	URLTypeSeed = iota
	URLTypeNew
	URLTypeResponse
	URLTypeRecover
)

// Link represents a link found by crawler.
type Link struct {
	URL       *url.URL // parsed url
	Text      []byte   // anchor text
	depth     int      // length of path to find it
	hyperlink bool     // is hyperlink?
}

// Client defines how requests are made.
type Client interface {
	Do(*Request) (*Response, error)
}

// Request is a HTTP request to be made.
type Request struct {
	*http.Request
	Proxy   *url.URL
	Cookies []*http.Cookie
	Client  Client
	ctx     *Context
}

func (r *Request) Context() *Context {
	return r.ctx
}

// Controller controls the working progress of crawler.
type Controller interface {
	// Prepare sets options(client, headers, ...) for a http request
	Prepare(req *Request)

	// Handle handles a response. If the content type of response is
	// text/html, the body of the response is prefetched. Some utils are
	// provided to handle html document. Handle can also extract
	// non-standard links from a response and return them. Note that it
	// doesn't need to handle standard links(<a href="..."></a>) in html
	// document because the crawler will do this.
	Handle(r *Response, ch chan<- *Link)

	// Schedule gives a score between 0 and 1024 for a URL, Higher score
	// means higher priority in queue. Schedule also specifies the next
	// time that this URL should be crawled at, but the crawling interval
	// will be respected at first. If this URL is expected to be not
	// crawled any more, return true for done.
	Schedule(ctx *Context, u *url.URL) (done bool, at time.Time, score int)

	// Accept determines whether a URL should be processed. It acts as a
	// blacklist that preventing some unneccesary URLs to be processed.
	Accept(ctx *Context, link *Link) bool

	// Interval gives the crawling interval of a site that the crawler should respect.
	Interval(host string) time.Duration

	// Charset determines the charset used by a HTML document.  It will be
	// called only when the crawler cannot determine the exact charset.
	Charset(u *url.URL) (label string)
}
