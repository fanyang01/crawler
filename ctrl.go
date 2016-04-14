package crawler

import (
	"net/url"
	"time"
)

// NopController is an empty controller - it walks through each seed once
// and does nothing.
type NopController struct{}

func (c NopController) Prepare(_ *Request)               {}
func (c NopController) Interval(_ string) time.Duration  { return 0 }
func (c NopController) Charset(_ *url.URL) string        { return "utf-8" }
func (c NopController) Follow(_ *Response, _ int) bool   { return false }
func (c NopController) Handle(_ *Response) []*Link       { return nil }
func (c NopController) Accept(_ *Response, _ *Link) bool { return true }
func (c NopController) Schedule(_ *URL, _ int, _ *Response) (done bool, at time.Time, score int) {
	return false, time.Time{}, 0
}

// OnceController visits each url once and follows urls found by crawler.
type OnceController struct{ NopController }

func (c OnceController) Schedule(u *URL, _ int, _ *Response) (done bool, at time.Time, score int) {
	if u.Visited.Count > 0 {
		return true, time.Time{}, 0
	}
	return false, time.Now(), 0
}
