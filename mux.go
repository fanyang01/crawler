package crawler

import (
	"net/url"

	"github.com/fanyang01/glob"
)

// Muxer serves requests that come from the query channel, replies handler to them.
type Muxer interface {
	Serve(<-chan *HandlerQuery)
}

// Mux is a multiplexer that supports wildcard *.
type Mux struct {
	trie *glob.Trie
}

// HandlerQuery querys handler for specific URL.
type HandlerQuery struct {
	URL   *url.URL
	Reply chan Handler
}

// NewMux creates a new multiplexer.
func NewMux() *Mux { return &Mux{trie: glob.NewTrie()} }

// Add adds a pattern and corresponding handler to multiplexer.
func (mux Mux) Add(pattern string, h Handler) {
	mux.trie.Add(pattern, h)
}

// Lookup searchs a pattern that most precisely matches s,
// and returns the corresponding handler.
func (mux Mux) Lookup(s string) (Handler, bool) {
	v, ok := mux.trie.Lookup(s)
	if !ok {
		return nil, false
	}
	return v.(Handler), true
}

// Serve implements Muxer.
func (mux Mux) Serve(query <-chan *HandlerQuery) {
	for q := range query {
		h, ok := mux.Lookup(q.URL.String())
		if !ok {
			q.Reply <- DefaultHandler
			continue
		}
		q.Reply <- h
	}
}
