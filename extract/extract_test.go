package extract

import (
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

func TestCompile(t *testing.T) {
	assert := assert.New(t)
	p := &Pattern{
		Accept:      []string{`**`, `/.*/`},
		Reject:      []string{`/.*\.(doc|xls|ppt|pdf)/`},
		Host:        []string{"*.google.com"},
		ExcludeHost: []string{"example.com"},
		Dir:         []string{`\/doc/`},
		File:        []string{"*.txt", `/.*\.html/`, "*.pdf", ""},
		ExcludeFile: []string{"*.mp3"},
	}
	m, err := Compile(p)
	assert.NoError(err)
	for _, v := range []struct {
		s  string
		ok bool
	}{
		{"http://www.example.com", false},
		{"http://example.com/doc/hello.txt", false},
		{"http://www.google.com/doc/", true},
		{"http://google.com", false},
		{"http://www.google.com/doc/hello.pdf", false},
		{"http://www.google.com/hello.html", false},
		{"http://www.google.com/doc/hello.html", true},
		{"http://www.google.com/doc/hello.mp3", false},
	} {
		assert.Equal(v.ok, m.Match(mustParse(v.s)), "%s", v.s)
	}
}
