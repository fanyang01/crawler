package crawler

import "time"

// NopController is an empty controller - it walks through each seed once
// and does nothing.
type NopController struct{}

func (c NopController) Prepare(_ *Request) {}
func (c NopController) Handle(_ *Response) (bool, []*Link) {
	return false, nil
}
func (c NopController) Interval(_ string) time.Duration { return 0 }
func (c NopController) Schedule(_ *URL) (score int, at time.Time, done bool) {
	return 0, time.Time{}, false
}
func (c NopController) Accept(_ *Link) bool { return true }

// OnceController visits each url once and follows urls found by crawler.
type OnceController struct{ NopController }

func (c OnceController) Schedule(u *URL) (score int, at time.Time, done bool) {
	if u.Visited.Count > 0 {
		return 0, time.Time{}, true
	}
	return 0, time.Now(), false
}
