package tree

import (
	"fmt"
	"testing"
)

func TestSiteTree(t *testing.T) {
	testIn := []string{
		"*abcd*ef*",
		"*.google.com",
		"http://example.com/books/*",
		"*://example.com/movies",
		"http://example.com/",
	}
	test := []struct {
		s     string
		found bool
	}{
		{"abcdef", true},
		{"abcdefef", true},
		{"abcabcdefgef", true},
		{"google.com", false},
		{"www.google.com", true},
		{"http://example.com/books/", true},
	}

	tr := &Tree{}
	for _, s := range testIn {
		tr.Insert(s, nil)
		// fmt.Println("After insert", s)
		// printSibling(tr.root)
	}

	for _, data := range test {
		if found := tr.Lookup(data.s); (found != nil) != data.found {
			t.Errorf("lookup %q failed: expect %v, got %v\n", data.s, data.found, !data.found)
		}
	}
}

func printSibling(node *Node) {
	fmt.Printf("%s: ", node.s)
	for _, n := range node.child {
		fmt.Printf("%s ", n.s)
	}
	fmt.Println("")
	for _, n := range node.child {
		printSibling(n)
	}
}
