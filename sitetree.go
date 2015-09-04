package crawler

import (
	"net/url"
	"path"
	"strings"
)

type SiteTree struct {
	host string
	root *Node
}

type Node struct {
	Child    *Node
	Siblings *Node
	Value    string
	Ending   bool
	URL      []url.URL // a database set should be used instead.
}

func NewSiteTree(host string) *SiteTree {
	return &SiteTree{
		host: host,
	}
}

func (st *SiteTree) Insert(u url.URL) (*Node, bool) {
	if st.host != u.Host {
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
			node = &Node{
				Value: section,
			}
			*pp = node
		} else if node.Value != section {
			for node != nil && node.Value < section {
				pp, node = &node.Siblings, node.Siblings
			}
			if node == nil || node.Value != section {
				node = &Node{
					Value:    section,
					Siblings: node,
				}
				*pp = node
			}
		}
		if i != len(sections)-1 {
			pp, node = &node.Child, node.Child
		}
	}
	node.Ending = true
	node.URL = append(node.URL, u)
	return node, true
}

func (st *SiteTree) Search(u url.URL) (node *Node, appear bool) {
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
				return nil, false
			}
		}
		if i != len(sections)-1 {
			node = node.Child
		}
	}
	return node, true
}

func (st *SiteTree) Contain(u url.URL) (yes bool) {
	var node *Node
	node, yes = st.Search(u)
	yes = yes && node.Ending
	return
}
