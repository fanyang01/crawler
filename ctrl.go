package crawler

import (
	"context"
	"net/url"
	"time"
)

type Ticket struct {
	At    time.Time
	Score int
	Ctx   context.Context
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
	Handle(r *Response, ch chan<- *url.URL)

	// Accept determines whether a URL should be processed. It acts as a
	// blacklist that preventing some unneccesary URLs to be processed.
	Accept(r *Response, u *url.URL) bool

	// Schedule gives a score between 0 and 1024 for a URL, Higher score
	// means higher priority in queue. Schedule also specifies the next
	// time that this URL should be crawled at, but the crawling interval
	// will be respected at first. If this URL is expected to be not
	// crawled any more, return true for done.
	Sched(r *Response, u *url.URL) Ticket

	Resched(r *Response) (done bool, t Ticket)

	// Interval gives the crawling interval of a site that the crawler should respect.
	Interval(host string) time.Duration

	// Charset determines the charset used by a HTML document.  It will be
	// called only when the crawler cannot determine the exact charset.
	Charset(u *url.URL) (label string)
}

// NopController is an empty controller - it walks through each seed once
// and does nothing.
type NopController struct{}

func (c NopController) Prepare(_ *Request)                    {}
func (c NopController) Interval(_ string) time.Duration       { return 0 }
func (c NopController) Charset(_ *url.URL) string             { return "utf-8" }
func (c NopController) Handle(_ *Response, _ chan<- *url.URL) {}
func (c NopController) Accept(_ *Response, _ *url.URL) bool   { return true }
func (c NopController) Sched(_ *Response, _ *url.URL) Ticket {
	return Ticket{}
}
func (c NopController) Resched(_ *Response) (bool, Ticket) {
	return true, Ticket{}
}
