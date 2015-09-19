package crawler

import "time"

type Controller interface {
	// Score gives a score between 0 and 1024 for a URL:
	// <= 0 means this URL will not be enqueued,
	// >= 1024 will be treat as 1024
	// A URL with score (0, 1024] will be enqueued. Higher score means higher
	// priority in queue.
	// Score also specifies the next time that this URL should be crawled at.
	Score(u *URL) (score int64, at time.Time)
	// Handle handles a response or a HTML document. If the content type of
	// response is text/html, the body of the response is prefetched and doc
	// will be non-nil. But before using doc.Tree or doc.SubURLs, a semaphore
	// from doc.TreeReady or doc.SubURLsReady should be recieved.
	Handle(resp *Response, doc *Doc)
}
