package crawler

import (
	"net/url"
	"time"

	"golang.org/x/net/context"
)

type Ticket struct {
	At    time.Time
	Score int
	Ctx   context.Context
}

// Controller controls the working progress of crawler.
type Controller interface {
	// Prepare sets options(client, headers, ...) for a http request.
	Prepare(req *Request)

	// Handle handles a response(writing to disk/DB, ...). Handle should
	// also extract hyperlinks from the response and send them to the
	// channel. Note that r.NewURL may differ from r.URL if r.URL has been
	// redirected, so r.NewURL should also be included if following
	// redirects is expected.
	Handle(r *Response, ch chan<- *url.URL)

	// Accept determines whether a URL should be processed. It is redundant
	// because you can do this in Handle, but it is provided for
	// convenience. It acts as a filter that prevents some unneccesary URLs
	// to be processed.
	Accept(r *Response, u *url.URL) bool

	// Sched issues a ticket for a new URL. The ticket specifies the next
	// time that this URL should be crawled at.
	Sched(r *Response, u *url.URL) Ticket

	// Resched is like Sched, but for URLs that have been crawled at least
	// one time. If r.URL is expected to be not crawled any more, return
	// true for done.
	Resched(r *Response) (done bool, t Ticket)

	// Retry gives the delay to retry and the maxmium number of retries.
	Retry(c *Context) (delay time.Duration, max int)

	Etc
}

// Etc provides additional information for crawler.
type Etc interface {
	// Interval gives the crawling interval of a site that the crawler
	// should respect.
	Interval(host string) time.Duration
	// Charset determines the charset used by a HTML document.  It will be
	// called only when the crawler cannot determine the exact charset.
	Charset(u *url.URL) (label string)
}

// NopController is an empty controller - it walks through each seed once
// and does nothing.
type NopController struct{}

func (c NopController) Prepare(_ *Request)                    {}
func (c NopController) Handle(_ *Response, _ chan<- *url.URL) {}
func (c NopController) Accept(_ *Response, _ *url.URL) bool   { return true }
func (c NopController) Sched(_ *Response, _ *url.URL) Ticket {
	return Ticket{}
}
func (c NopController) Resched(_ *Response) (bool, Ticket) {
	return true, Ticket{}
}
func (c NopController) Retry(_ *Context) (time.Duration, int) {
	return 10 * time.Second, 4
}
func (c NopController) Interval(_ string) time.Duration { return 0 }
func (c NopController) Charset(_ *url.URL) string       { return "utf-8" }
