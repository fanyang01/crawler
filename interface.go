package crawler

import "time"

// Handler controls the working process of crawler.
type Handler interface {
	Scheduler
	Setter
	Filter
	Reciever
}

type Scheduler interface {
	// Schedule gives a score between 0 and 1024 for a URL, Higher score means
	// higher priority in queue.  Schedule also specifies the next time that
	// this URL should be crawled at. If this URL is expected to be not crawled
	// any more, return true for done.
	Schedule(u URL) (score int64, at time.Time, done bool)
}

type Setter interface {
	// SetRequest sets options(client, headers, ...) for a http request
	SetRequest(*Request)
}

type Filter interface {
	// Accept determines whether a URL should be processed. It acts as a
	// blacklist that preventing some unneccesary URLs to be processed.
	Accept(anchor Anchor) bool
}

type Reciever interface {
	// Recieve handles a response. If the content type of
	// response is text/html, the body of the response is prefetched. If the HTML tree of doc is needed,
	// resp.ParseHTML() should be called explicitly because it may result in
	// many allocations.
	Recieve(resp *Response) bool
}

// Client does request.
type Client interface {
	Do(*Request) (*Response, error)
}
