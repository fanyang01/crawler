package crawler

import (
	"net/url"
	"path"
	"strings"
)

type SiteTree struct {
	host   string
	scheme string // http, https
	root   *SiteNode
}

type SiteNode struct {
	Child    *SiteNode
	Siblings *SiteNode
	Value    string
	RawQuery map[string]bool
	Fragment map[string]bool
	End      bool
}

var EmptyValue struct{}

func NewSiteTree(scheme, host string) *SiteTree {
	return &SiteTree{
		scheme: scheme,
		host:   host,
	}
}

func (st *SiteTree) Insert(u url.URL) (*SiteNode, bool) {
	if st.scheme != u.Scheme || st.host != u.Host {
		return nil, false
	}

	pth := path.Clean(u.RawPath) // only for go1.5
	// treat "example.com" as "example.com/"
	if pth == "." {
		pth = "/"
	}

	// Split("/search", "/") = ["" "search"]
	sections := strings.Split(pth, "/")

	pp, node := &st.root, st.root
	for i, section := range sections {
		if node == nil {
			node = &SiteNode{
				Value:    section,
				RawQuery: make(map[string]bool),
				Fragment: make(map[string]bool),
			}
			*pp = node
		} else if node.Value != section {
			for node != nil && node.Value < section {
				pp, node = &node.Siblings, node.Siblings
			}
			if node == nil || node.Value != section {
				node = &SiteNode{
					Value:    section,
					Siblings: node,
					RawQuery: make(map[string]bool),
					Fragment: make(map[string]bool),
				}
				*pp = node
			}
		}
		if i != len(sections)-1 {
			pp, node = &node.Child, node.Child
		}
	}
	node.RawQuery[u.Query().Encode()] = true
	node.Fragment[u.Fragment] = true
	node.End = true
	return node, true
}

func (st *SiteTree) Search(u url.URL) (node *SiteNode) {
	if st.host != u.Host {
		return
	}

	pth := path.Clean(u.RawPath) // only for go1.5
	if pth == "." {
		pth = "/"
	}

	sections := strings.Split(pth, "/")

	node = st.root
	for i, section := range sections {
		if node == nil {
			return
		} else if node.Value != section {
			for node != nil && node.Value < section {
				node = node.Siblings
			}
			if node == nil || node.Value != section {
				return nil
			}
		}
		if i != len(sections)-1 {
			node = node.Child
		}
	}
	return node
}

func (st *SiteTree) Contain(u url.URL) bool {
	node := st.Search(u)
	return node != nil && node.End &&
		node.RawQuery[u.Query().Encode()] && node.Fragment[u.Fragment]
}
