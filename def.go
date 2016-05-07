// Package crawler provides a flexible web crawler.
package crawler

import (
	"net/http"
	"net/url"
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

// LinkPerPage is the rouge number of links in a HTML document.
const LinkPerPage = 32

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
