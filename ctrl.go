package crawler

import (
	"net/url"
	"time"
)

// NopController is an empty controller - it walks through each seed once
// and does nothing.
type NopController struct{}

func (c NopController) Prepare(_ *Request)                 {}
func (c NopController) Interval(_ string) time.Duration    { return 0 }
func (c NopController) Charset(_ *url.URL) string          { return "utf-8" }
func (c NopController) Handle(_ *Response, _ chan<- *Link) {}
func (c NopController) Accept(_ *Context, _ *Link) bool    { return true }
func (c NopController) Schedule(_ *Context, _ *url.URL) (done bool, at time.Time, score int) {
	return false, time.Time{}, 0
}

// OnceController visits each url once and follows urls found by crawler.
type OnceController struct{ NopController }

func (c OnceController) Schedule(ctx *Context, _ *url.URL) (done bool, at time.Time, score int) {
	if cnt, _ := ctx.VisitCount(); cnt > 0 {
		return true, time.Time{}, 0
	}
	return false, time.Now(), 0
}
