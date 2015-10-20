package crawler

import "time"

// Controller controls the working process of crawler.
type Controller interface {
	// Prepare sets options(client, headers, ...) for a http request
	Prepare(*Request)
	// Schedule gives a score between 0 and 1024 for a URL, Higher score means
	// higher priority in queue.  Schedule also specifies the next time that
	// this URL should be crawled at. If this URL is expected to be not crawled
	// any more, return true for done.
	Schedule(u URL) (score int64, at time.Time, done bool)
	// Accept determines whether a URL should be processed. It acts as a
	// blacklist that preventing some unneccesary URLs to be processed.
	Accept(anchor Anchor) bool
	// Handle handles a response. If the content type of
	// response is text/html, the body of the response is prefetched. Some
	// utils are provided to handle html document.
	Handle(resp *Response) bool
}

// Client does request.
type Client interface {
	Do(*Request) (*Response, error)
}
