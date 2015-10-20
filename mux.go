package crawler

import (
	"time"

	"github.com/fanyang01/glob"
)

const (
	mux_FILTER = 1 << iota
	mux_PREPARE
	mux_HANDLE
	mux_SCHED
)

type (
	Preparer interface {
		Prepare(*Request)
	}
	Handler interface {
		Handle(*Response) (follow bool)
	}
	Scheduler interface {
		Schedule(URL) (score int64, at time.Time, done bool)
	}
	PreparerFunc  func(*Request)
	HandlerFunc   func(*Response) bool
	SchedulerFunc func(URL) (score int64, at time.Time, done bool)
)

// Once returns a scheduler that schedule each url to be crawled only once.
func Once() SchedulerFunc {
	return func(u URL) (score int64, at time.Time, done bool) {
		if u.Visited.Count > 0 {
			done = true
		} else {
			done = false
		}
		return
	}
}

// Every returns a scheduler that schedule each url to be crawled every delta duration.
func Every(delta time.Duration) SchedulerFunc {
	return func(u URL) (score int64, at time.Time, done bool) {
		at = u.Visited.Time.Add(delta)
		return
	}
}

func (f PreparerFunc) Prepare(req *Request)      { f(req) }
func (f HandlerFunc) Handle(resp *Response) bool { return f(resp) }
func (f SchedulerFunc) Schedule(u URL) (score int64, at time.Time, done bool) {
	return f(u)
}

type processMux struct {
	tries map[int]*glob.Trie
}

// Mux is a multiplexer that supports wildcard *.
type Mux struct {
	processMux
}

// DefaultMux is the default multiplexer.
var DefaultMux = NewMux()

// NewMux creates a new multiplexer.
func NewMux() *Mux {
	return &Mux{
		processMux: processMux{
			tries: map[int]*glob.Trie{
				mux_FILTER:  glob.NewTrie(),
				mux_PREPARE: glob.NewTrie(),
				mux_HANDLE:  glob.NewTrie(),
				mux_SCHED:   glob.NewTrie(),
			},
		},
	}
}

// Prepare registers a handler to set requests for urls matching pattern.
func (mux *Mux) PrepareFor(pattern string, p Preparer) {
	mux.tries[mux_PREPARE].Add(pattern, p)
}

// PrepareFunc registers a function to set requests for urls matching pattern.
func (mux *Mux) PrepareFunc(pattern string, f func(*Request)) {
	mux.PrepareFor(pattern, PreparerFunc(f))
}

// Handle registers r to handle responses for urls matching pattern.
func (mux *Mux) HandleResp(pattern string, h Handler) {
	mux.tries[mux_HANDLE].Add(pattern, h)
}

// HandleFunc registers a function to handle responses for urls matching pattern.
func (mux *Mux) HandleFunc(pattern string, f func(*Response) bool) {
	mux.HandleResp(pattern, HandlerFunc(f))
}

// Sched registers sched to schedule urls matching pattern.
func (mux *Mux) Sched(pattern string, sched Scheduler) {
	mux.tries[mux_SCHED].Add(pattern, sched)
}

// SchedFunc registers a function to schedule urls matching pattern.
func (mux *Mux) SchedFunc(pattern string,
	f func(URL) (score int64, at time.Time, done bool)) {
	mux.Sched(pattern, SchedulerFunc(f))
}

// Sift is a filter to determine whether a url should be processed.
func (mux *Mux) Sift(pattern string, accept bool) {
	mux.tries[mux_FILTER].Add(pattern, accept)
}

// Prepare implements Handler.
func (mux *processMux) Prepare(req *Request) {
	if f, ok := mux.tries[mux_PREPARE].Lookup(req.URL.String()); ok {
		f.(Preparer).Prepare(req)
	}
}

// Handle implements Handler.
func (mux *processMux) Handle(resp *Response) bool {
	if f, ok := mux.tries[mux_HANDLE].Lookup(resp.Locations.String()); ok {
		return f.(Handler).Handle(resp)
	}
	return true
}

// Schedule implements Handler.
func (mux *processMux) Schedule(u URL) (score int64, at time.Time, done bool) {
	if f, ok := mux.tries[mux_SCHED].Lookup(u.Loc.String()); ok {
		return f.(Scheduler).Schedule(u)
	}
	return
}

// Accept implements Handler.
func (mux *processMux) Accept(anchor Anchor) bool {
	if ac, ok := mux.tries[mux_FILTER].Lookup(anchor.URL.String()); ok {
		return ac.(bool)
	}
	return false
}
