package crawler

import (
	"net/url"

	"github.com/fanyang01/glob"
)

type router struct {
	*glob.Trie
}

type ctrlQuery struct {
	url   *url.URL
	reply chan Controller
}

func newRouter() *router { return &router{glob.New()} }

func (r router) Add(pattern string, ctrl Controller) {
	r.Trie.Add(pattern, ctrl)
}

func (r router) Lookup(s string) (Controller, bool) {
	v, ok := r.Trie.Lookup(s)
	if !ok {
		return nil, false
	}
	return v.(Controller), true
}

func (r router) Serve(query <-chan ctrlQuery) {
	for q := range query {
		ctrl, ok := r.Lookup(q.url.String())
		if !ok {
			q.reply <- DefaultController
			continue
		}
		q.reply <- ctrl
	}
}
