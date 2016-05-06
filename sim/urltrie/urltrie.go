package urltrie

import (
	"net/url"
	"sort"
	"strings"
)

type (
	kv        struct{ k, v string }
	dictOrder []kv
)

func (a dictOrder) Len() int      { return len(a) }
func (a dictOrder) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a dictOrder) Less(i, j int) bool {
	if a[i].k < a[j].k {
		return true
	} else if a[i].k > a[j].k {
		return false
	}
	return a[i].v < a[j].v
}
func sorted(query url.Values) []kv {
	var r []kv
	for k, vv := range query {
		for _, v := range vv {
			r = append(r, kv{k, v})
		}
	}
	sort.Sort(dictOrder(r))
	return r
}

type QueryNode struct {
	next map[string]map[string]*QueryNode
}

func newQueryNode() *QueryNode { return &QueryNode{} }

type PathNode struct {
	child map[string]*PathNode // key: segment
	query map[string]map[string]*QueryNode
}

func newPathNode() *PathNode { return &PathNode{} }

// A Trie is a special trie for URL. It reuses nodes at the segment level.
// For instance, https://golang.org will be stored as:
//
//              ""
//         /   /  \   \
//        doc pkg  src help ...
//          /  |  \
//        net fmt flag ...
type Trie struct{ root PathNode }

// New creates a url trie.
func New() *Trie { return &Trie{} }

// Add adds a URL to the trie. It will cancel and return false if the
// number of children of some node on the path exceeds the threshold.
func (t *Trie) Add(u *url.URL, threshold int) bool {
	var (
		pnode    = &t.root
		segments = strings.Split(u.EscapedPath(), "/")
		m        map[string]*PathNode
		prev     string
		ok       bool
	)
	for _, seg := range segments[1:] {
		if pnode == nil {
			pnode = newPathNode()
			m[prev] = pnode
		}
		if m = pnode.child; m == nil {
			m = make(map[string]*PathNode, 1)
			pnode.child = m
		}
		if pnode, ok = m[seg]; !ok {
			if threshold > 0 && len(pnode.child) >= threshold {
				return false
			}
			m[seg] = nil
		}
		prev = seg
	}

	query := sorted(u.Query())
	if len(query) == 0 {
		return true
	} else if pnode == nil {
		pnode = newPathNode()
		m[prev] = pnode
	} // DON'T use 'else if'!
	if pnode.query == nil {
		pnode.query = make(map[string]map[string]*QueryNode, 1)
	}

	var (
		primary   = pnode.query
		qnode     = &QueryNode{next: primary}
		secondary map[string]*QueryNode
	)
	for _, kv := range query {
		if qnode == nil {
			qnode = newQueryNode()
			secondary[prev] = qnode
		}
		if primary = qnode.next; primary == nil {
			primary = make(map[string]map[string]*QueryNode, 1)
			qnode.next = primary
		}
		if secondary = primary[kv.k]; secondary == nil {
			secondary = make(map[string]*QueryNode, 1)
			primary[kv.k] = secondary
		}
		if qnode, ok = secondary[kv.v]; !ok {
			if threshold > 0 && len(secondary) >= threshold {
				return false
			}
			secondary[kv.v] = nil
		}
		prev = kv.v
	}
	return true
}

// Has searches the trie to check whether there are similar URLs. It will
// return true either the number of children of some node on the lookup
// path is greater than or equal to the threshold, or an exact match is found.
func (t *Trie) Has(u *url.URL, threshold int) bool {
	pnode := &t.root
	segments := strings.Split(u.EscapedPath(), "/")
	// Consider github.com/{user}. If the number of users is equal to
	// threshold, github.com/someone-stored/{repo} should still be enabled.
	for _, seg := range segments[1:] {
		if pnode == nil || pnode.child == nil {
			return false
		}
		child, ok := pnode.child[seg]
		if !ok {
			if threshold > 0 && len(pnode.child) >= threshold {
				return true
			}
			return false
		}
		pnode = child
	}

	query := sorted(u.Query())
	if len(query) == 0 {
		return true
	} else if pnode == nil {
		return false
	}
	primary := pnode.query
	qnode := &QueryNode{next: primary}

	for _, kv := range query {
		if qnode == nil {
			return false
		} else if primary = qnode.next; primary == nil {
			return false
		}
		secondary := primary[kv.k]
		if secondary == nil {
			return false
		}
		var ok bool
		qnode, ok = secondary[kv.v]
		if !ok {
			if threshold > 0 && len(secondary) >= threshold {
				return true
			}
			return false
		}
	}
	// Totally match
	return true
}
