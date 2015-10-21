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
		Schedule(*URL) (score int, at time.Time, done bool)
	}
	PreparerFunc  func(*Request)
	HandlerFunc   func(*Response) bool
	SchedulerFunc func(*URL) (score int, at time.Time, done bool)
)

// Once returns a scheduler that schedule each url to be crawled only once.
func Once() Scheduler {
	return SchedulerFunc(func(u *URL) (score int, at time.Time, done bool) {
		if u.Visited.Count > 0 {
			done = true
		} else {
			done = false
		}
		return
	})
}

// Every returns a scheduler that schedule each url to be crawled every delta duration.
func Every(delta time.Duration) Scheduler {
	return SchedulerFunc(func(u *URL) (score int, at time.Time, done bool) {
		at = u.Visited.Time.Add(delta)
		return
	})
}

func (f PreparerFunc) Prepare(req *Request)      { f(req) }
func (f HandlerFunc) Handle(resp *Response) bool { return f(resp) }
func (f SchedulerFunc) Schedule(u *URL) (score int, at time.Time, done bool) {
	return f(u)
}

// Mux is a multiplexer that supports wildcard *.
type Mux struct {
	tries map[int]*glob.Trie
}

// NewMux creates a new multiplexer.
func NewMux() *Mux {
	return &Mux{
		tries: map[int]*glob.Trie{
			mux_FILTER:  glob.NewTrie(),
			mux_PREPARE: glob.NewTrie(),
			mux_HANDLE:  glob.NewTrie(),
			mux_SCHED:   glob.NewTrie(),
		},
	}
}

// AddPreparer egisters p  to set requests for urls matching pattern.
func (mux *Mux) AddPreparer(pattern string, p Preparer) {
	mux.tries[mux_PREPARE].Add(pattern, p)
}

// AddPrepareFunc registers a function to set requests for urls matching pattern.
func (mux *Mux) AddPrepareFunc(pattern string, f func(*Request)) {
	mux.AddPreparer(pattern, PreparerFunc(f))
}

// AddHandler registers h to handle responses for urls matching pattern.
func (mux *Mux) AddHandler(pattern string, h Handler) {
	mux.tries[mux_HANDLE].Add(pattern, h)
}

// AddHandleFunc registers a function to handle responses for urls matching pattern.
func (mux *Mux) AddHandleFunc(pattern string, f func(*Response) bool) {
	mux.AddHandler(pattern, HandlerFunc(f))
}

// AddScheduler registers sched to schedule urls matching pattern.
func (mux *Mux) AddScheduler(pattern string, sched Scheduler) {
	mux.tries[mux_SCHED].Add(pattern, sched)
}

// AddScheduleFunc registers a function to schedule urls matching pattern.
func (mux *Mux) AddScheduleFunc(pattern string,
	f func(*URL) (score int, at time.Time, done bool)) {
	mux.AddScheduler(pattern, SchedulerFunc(f))
}

// Sift determines whether a url should be processed.
func (mux *Mux) Sift(pattern string, accept bool) {
	mux.tries[mux_FILTER].Add(pattern, accept)
}

// Prepare implements Controller.
func (mux *Mux) Prepare(req *Request) {
	if f, ok := mux.tries[mux_PREPARE].Lookup(req.URL.String()); ok {
		f.(Preparer).Prepare(req)
	}
}

// Handle implements Controller.
func (mux *Mux) Handle(resp *Response) bool {
	if f, ok := mux.tries[mux_HANDLE].Lookup(resp.Locations.String()); ok {
		return f.(Handler).Handle(resp)
	}
	return true
}

// Schedule implements Controller.
func (mux *Mux) Schedule(u *URL) (score int, at time.Time, done bool) {
	if f, ok := mux.tries[mux_SCHED].Lookup(u.Loc.String()); ok {
		return f.(Scheduler).Schedule(u)
	}
	return
}

// Accept implements Controller.
func (mux *Mux) Accept(anchor Anchor) bool {
	if ac, ok := mux.tries[mux_FILTER].Lookup(anchor.URL.String()); ok {
		return ac.(bool)
	}
	return false
}
