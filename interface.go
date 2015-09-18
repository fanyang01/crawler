package crawler

type Controller interface {
	// Score gives a score between 0 and 1024 for a URL:
	// <= 0 means this URL will not be enqueued,
	// >= 1024 will be treat as 1024
	// A URL with score (0, 1024] will be enqueued. Higher score means higher priority in queue.
	Score(u *URL) int64
	// HandleResponse handles a response. The body of the response may be prefetched.
	Handle(*Response, *Doc)
	// Log(...interface{})
}
