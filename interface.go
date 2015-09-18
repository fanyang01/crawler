package crawler

type Controller interface {
	// Score gives a score between 0 and 1024 for a URL:
	// <= 0 means this URL will not be enqueued,
	// >= 1024 will be treat as 1024
	// A URL with score (0, 1024] will be enqueued. Higher score means higher priority in queue.
	Score(u *URL) int64
	// Handle handles a response or a HTML document. If the content type of
	// response is text/html, the body of the response is prefetched and doc
	// will be non-nil. But before using doc.Tree or doc.SubURLs, a semaphore
	// from doc.TreeReady or doc.SubURLsReady should be recieved.
	Handle(resp *Response, doc *Doc)
}
