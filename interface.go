package crawler

import (
	"net/url"
	"time"
)

type Controller interface {
	// Accept determines whether a URL should be processed. It acts as a
	// blacklist that preventing some unneccesary URLs to be processed,
	// so to save resources(CPU, memory, ...).
	Accept(u url.URL) bool

	// Schedule gives a score between 0 and 1024 for a URL:
	// <= 0 means this URL will not be enqueued,
	// >= 1024 will be treat as 1024
	// A URL with score (0, 1024] will be enqueued. Higher score means higher
	// priority in queue.  Schedule also specifies the next time that this URL
	// should be crawled at.
	// It's recommended to use the fields of u.Loc rather than u.Loc.String()
	// to avoid extra space allocation.
	Schedule(u URL) (score int64, at time.Time)

	// Handle handles a response or a HTML document. If the content type of
	// response is text/html, the body of the response is prefetched and doc
	// will be non-nil. But before using doc.SubURLs, a semaphore from
	// doc.SubURLsReady should be recieved. If the HTML tree of doc is needed,
	// doc.ParseHTML() should be called explicitly because it may result in
	// many allocations.
	Handle(resp *Response)

	// SetRequest sets options(client, headers, ...) for a http request
	SetRequest(*Request)
}

type Client interface {
	Do(*Request) (*Response, error)
}
