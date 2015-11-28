package crawler

import "time"

// NopController is an empty controller - it walks through each seed once and does nothing.
type NopController struct{}

func (c NopController) Prepare(_ *Request) {}
func (c NopController) Handle(resp *Response) bool {
	return true
}
func (c NopController) Schedule(u *URL) (score int, at time.Time, done bool) {
	return 0, time.Time{}, true
}
func (c NopController) Accept(_ *Anchor) bool { return true }

// OnceController visits each url once and follows urls found by crawler.
type OnceController struct{ NopController }

func (c OnceController) Schedule(u *URL) (score int, at time.Time, done bool) {
	if u.Visited.Count > 0 {
		return 0, time.Time{}, true
	}
	return 0, time.Now(), false
}
