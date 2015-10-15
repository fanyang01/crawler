package crawler

import (
	"time"

	"github.com/fanyang01/glob"
)

const (
	mux_FILTER = 1 << iota
	mux_SET
	mux_RECV
	mux_SCHED
)

type (
	SetterFunc    func(*Request)
	RecieverFunc  func(*Response) bool
	SchedulerFunc func(URL) (score int64, at time.Time, done bool)
)

func (f SetterFunc) SetRequest(req *Request)       { f(req) }
func (f RecieverFunc) Recieve(resp *Response) bool { return f(resp) }
func (f SchedulerFunc) Schedule(u URL) (score int64, at time.Time, done bool) {
	return f(u)
}

// Mux is a multiplexer that supports wildcard *.
type Mux struct {
	tries map[int]*glob.Trie
}

// DefaultMux is the default multiplexer.
var DefaultMux = NewMux()

// NewMux creates a new multiplexer.
func NewMux() *Mux {
	return &Mux{
		tries: map[int]*glob.Trie{
			mux_FILTER: glob.NewTrie(),
			mux_SET:    glob.NewTrie(),
			mux_RECV:   glob.NewTrie(),
			mux_SCHED:  glob.NewTrie(),
		},
	}
}

// Set registers setter to set requests for urls matching pattern.
func (mux *Mux) Set(pattern string, setter Setter) {
	mux.tries[mux_SET].Add(pattern, setter)
}

// SetFunc registers a function to set requests for urls matching pattern.
func (mux *Mux) SetFunc(pattern string, f func(*Request)) {
	mux.Set(pattern, SetterFunc(f))
}

// Recv registers r to recieve and handle responses for urls matching pattern.
func (mux *Mux) Recv(pattern string, r Reciever) {
	mux.tries[mux_RECV].Add(pattern, r)
}

// RecvFunc registers a function to recieve and handle responses for urls matching pattern.
func (mux *Mux) RecvFunc(pattern string, f func(*Response) bool) {
	mux.Recv(pattern, RecieverFunc(f))
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

// Sieve is a filter to determine whether a url should be processed.
func (mux *Mux) Sieve(pattern string, accept bool) {
	mux.tries[mux_FILTER].Add(pattern, accept)
}

// SetRequest implements Setter.
func (mux *Mux) SetRequest(req *Request) {
	if f, ok := mux.tries[mux_SET].Lookup(req.URL.String()); ok {
		f.(Setter).SetRequest(req)
	}
}

// Recieve implements Reciever.
func (mux *Mux) Recieve(resp *Response) bool {
	if f, ok := mux.tries[mux_RECV].Lookup(resp.Locations.String()); ok {
		return f.(Reciever).Recieve(resp)
	}
	return true
}

// Schedule implements Scheduler.
func (mux *Mux) Schedule(u URL) (score int64, at time.Time, done bool) {
	if f, ok := mux.tries[mux_SCHED].Lookup(u.Loc.String()); ok {
		return f.(Scheduler).Schedule(u)
	}
	return 0, time.Time{}, false
}

// Accept implements Filter.
func (mux *Mux) Accept(anchor Anchor) bool {
	if ac, ok := mux.tries[mux_FILTER].Lookup(anchor.URL.String()); ok {
		return ac.(bool)
	}
	return false
}
