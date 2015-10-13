package crawler

import "time"

// NopHandler is an empty handler - it walks through each seed once and does nothing.
type NopHandler struct{}

func (c NopHandler) Handle(resp *Response) bool {
	return true
}
func (c NopHandler) Schedule(u URL) (score int64, at time.Time, done bool) {
	return 0, time.Time{}, true
}
func (c NopHandler) Accept(_ Anchor) bool  { return true }
func (c NopHandler) SetRequest(_ *Request) {}

// OnceHandler visits each url once and follows urls found by crawler.
type OnceHandler struct{ NopHandler }

func (h OnceHandler) Schedule(u URL) (score int64, at time.Time, done bool) {
	if u.Visited.Count > 0 {
		return 0, time.Time{}, true
	}
	return 1024, time.Now(), false
}

var (
	DefaultHandler = &OnceHandler{}
)
