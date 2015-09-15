package crawler

import "net/url"

type Controller interface {
	// Score gives a score between 0 and 1.0 for a URL:
	// <= 0 means this URL will not be enqueue,
	// >= 1 will be treat as 1.0.
	// A URL with score (0, 1.0] will be enqueued. Higher score means higher
	// priority in queue.
	Score(u *url.URL) float64
	// HandleResponse handles a response. The body of the response may be prefetched.
	HandleResponse(*Response)
	// DoRequest performs HTTP request or other stuffs to return a response.
	DoRequest(*Request) (*Response, error)
	// Log(...interface{})
}
