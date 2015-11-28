package mux

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMatcher(t *testing.T) {
	assert := assert.New(t)
	m := NewMatcher()

	assert.Nil(m.Add("*", 0))
	assert.Nil(m.Add("*://example.org/*", 1))
	assert.Nil(m.Add("http://example.org/*", 2))
	assert.Nil(m.Add("http://example.org/section/*", 3))
	assert.Nil(m.Add("= http://example.org/", 4))
	assert.Nil(m.Add("~ http://example.org/section/hello/.*", 5))
	assert.Nil(m.Add("^~ http://example.org/foo/*", 6))
	assert.Nil(m.Add("~ http://example.org/foo/.*", 7))

	get := func(s string) interface{} {
		v, ok := m.Get(s)
		assert.True(ok)
		return v
	}

	assert.Equal(get("hello, world"), 0)
	assert.Equal(get("https://example.org/"), 1)
	assert.Equal(get("http://example.org/bar"), 2)
	assert.Equal(get("http://example.org/section"), 2)
	assert.Equal(get("http://example.org/section/"), 3)
	assert.Equal(get("http://example.org/"), 4)
	assert.Equal(get("http://example.org/section/hello/world"), 5)
	assert.Equal(get("http://example.org/foo/hello/world"), 6)
}
