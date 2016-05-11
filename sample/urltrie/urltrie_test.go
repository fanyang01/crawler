package urltrie

import (
	"fmt"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

func mustParse(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}
func TestAdd(t *testing.T) {
	assert := assert.New(t)
	trie := New()
	data := []string{
		"http://localhost/pkg/net/",
		"http://localhost/pkg/net/http/",
		"http://localhost/pkg/",
		"http://localhost/doc/",
		"http://localhost",
		"http://localhost/",
		"http://localhost/src/",
		"http://localhost/src/?page=1&id=1",
		"http://localhost/src/?page=1",
		"http://localhost/src/?id=1",
		"http://localhost/src/?id=2&page=1",
		"http://localhost?hello=world",
		"http://localhost/?hello=world",
	}
	for _, u := range data {
		assert.True(trie.Add(mustParse(u), nil), u)
		// print(trie)
	}
	for _, u := range data {
		assert.True(trie.Has(mustParse(u), nil), u)
	}
}

func TestThreshold(t *testing.T) {
	assert := assert.New(t)
	trie := New()
	gen := func(i int) func(int) int {
		return func(_ int) int {
			return i
		}
	}
	check := func(url string, threshold int, exp bool) {
		assert.Equal(exp, trie.Add(mustParse(url), gen(threshold)))
	}
	check("http://localhost/pkg/net/http/httptest", 1, true)
	check("http://localhost/pkg/net/url", 1, false)
	check("http://localhost/pkg/net/url", 2, true)
	check("http://localhost/pkg/net/hello", 2, false)
	check("http://localhost/pkg/net/url?hello=world", 2, true)
	check("http://localhost/pkg/net/url?hello=foo", 2, true)
	check("http://localhost/pkg/net/url?hello=bar", 2, false)
	check("http://localhost/pkg/net/url?foo=world", 2, true)
	check("http://localhost/pkg/net/url?bar=world", 2, false)
}

func print(t *Trie) {
	var walkQuery func(*QueryNode, string)
	walkQuery = func(n *QueryNode, s string) {
		if n == nil {
			fmt.Printf("%s ", s)
			return
		}
		for k, secondary := range n.next {
			for v, child := range secondary {
				walkQuery(
					child,
					fmt.Sprintf("%s&%s=%s", s, k, v),
				)
			}
		}
	}
	var walk func(*PathNode, int)
	walk = func(n *PathNode, depth int) {
		if n == nil {
			fmt.Println("$")
			return
		}
		for k, _ := range n.child {
			if k == "" {
				k = "/"
			}
			fmt.Printf("%s ", k)
		}
		if n.query != nil {
			qnode := &QueryNode{next: n.query}
			walkQuery(qnode, "?")
		}
		fmt.Println("")

		for k, n := range n.child {
			for i := 0; i < depth+1; i++ {
				fmt.Print("----")
			}
			if k == "" {
				k = "/"
			}
			fmt.Printf("%s: ", k)
			walk(n, depth+1)
		}
	}
	fmt.Print("[root]: ")
	walk(&t.root, 0)
}
