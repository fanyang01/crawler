package tree

import (
	"fmt"
	"testing"
)

func TestSiteTree(t *testing.T) {
	testIn := []struct {
		s string
		i interface{}
	}{
		{"*abcd*ef*", 1},
		{"*.google.com", 2},
		{"http://example.com/books/*", 3},
		{"*://example.com/movies", 4},
		{`http://example.com/\*`, 5},
		{`http://example.com/*`, 6},
	}
	test := []struct {
		s string
		v interface{}
	}{
		{"abcdef", 1},
		{"abcdefef", 1},
		{"abcabcdefgef", 1},
		{"google.com", nil},
		{"www.google.com", 2},
		{"http://example.com/books/", 3},
		{"http://example.com/", 6},
		{"http://example.com/*", 5},
	}

	tr := &Tree{}
	for _, s := range testIn {
		tr.Add(s.s, s.i)
	}

	for _, data := range test {
		// fmt.Println("Lookup", data.s)
		v, ok := tr.Lookup(data.s)
		if !ok && v != data.v {
			t.Errorf("lookup %q failed: expect %v, got %v", data.s, data.v, v)
		}
	}
}

func printSibling(node *Node) {
	fmt.Printf("%s: ", node.s)
	for _, n := range node.child {
		fmt.Printf("%s ", n.s)
	}
	if node.wcard != nil {
		fmt.Printf("*-")
	}
	fmt.Println("")
	for _, n := range node.child {
		printSibling(n)
	}
	if node.wcard != nil {
		printSibling(node.wcard)
	}
}
