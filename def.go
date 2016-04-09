// Package crawler provides a flexible web crawler.
package crawler

import (
	"net/http"
	"net/url"
	"time"
)

// RequestType defines the type of a request.
type RequestType int

const (
	// Static content that can be reached by simple HTTP method.
	ReqStatic RequestType = iota
	// Dynamic pages that require a browser to complete rendering.
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

// Link represents a link found by crawler.
type Link struct {
	URL       *url.URL // parsed url
	Text      []byte   // anchor text
	Depth     int      // length of path to find it
	Hyperlink bool     // is hyperlink?
	follow    bool
}

// Controller controls the working process of crawler.
type Controller interface {
	// Prepare sets options(client, headers, ...) for a http request
	Prepare(*Request)

	// Interval gives the crawling interval of a site that the crawler should respect.
	Interval(host string) time.Duration

	// Schedule gives a score between 0 and 1024 for a URL, Higher score
	// means higher priority in queue. Schedule also specifies the next
	// time that this URL should be crawled at, but the crawling interval
	// will be respected at first. If this URL is expected to be not
	// crawled any more, return true for done.
	Schedule(u *URL) (score int, at time.Time, done bool)

	// Accept determines whether a URL should be processed. It acts as a
	// blacklist that preventing some unneccesary URLs to be processed.
	Accept(link *Link) bool

	// Charset determines the charset used by a HTML document.
	// It will be called only when the crawler cannot determine the exact
	// charset.
	Charset(u *url.URL) (label string)

	// Follow determines whether the crawler should follow links in an HTML
	// document.
	Follow(u *url.URL, depth int) bool

	// Handle handles a response. If the content type of response is
	// text/html, the body of the response is prefetched. Some utils are
	// provided to handle html document. Handle can also extract
	// non-standard links from a response and return them. Note that it
	// doesn't need to handle standard links(<a href="..."></a>) in html
	// document because the crawler will do this.
	Handle(resp *Response) []*Link
}
