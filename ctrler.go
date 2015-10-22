package crawler

import "time"

// NopCtrler is an empty controller - it walks through each seed once and does nothing.
type NopCtrler struct{}

func (c NopCtrler) Prepare(_ *Request) {}
func (c NopCtrler) Handle(resp *Response) bool {
	return true
}
func (c NopCtrler) Schedule(u *URL) (score int, at time.Time, done bool) {
	return 0, time.Time{}, true
}
func (c NopCtrler) Accept(_ *Anchor) bool { return true }

// OnceCtrler visits each url once and follows urls found by crawler.
type OnceCtrler struct{ NopCtrler }

func (h OnceCtrler) Schedule(u *URL) (score int, at time.Time, done bool) {
	if u.Visited.Count > 0 {
		return 0, time.Time{}, true
	}
	return 0, time.Now(), false
}
