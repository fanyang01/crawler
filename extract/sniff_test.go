package extract

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWindowLocation(t *testing.T) {
	base := mustParse("http://example.com")
	us := windowLocation(
		base, `
window.location.href = "/doc/hello.txt" location.href= '/doc/hello.html'
`,
	)
	assert := assert.New(t)
	assert.Equal(2, len(us))
	assert.Equal(us[0].String(), "http://example.com/doc/hello.txt")
	assert.Equal(us[1].String(), "http://example.com/doc/hello.html")
}

func TestAbsoluteURLs(t *testing.T) {
	base := mustParse("http://example.com")
	us := absoluteURLs(
		base, `
http://example.com/doc/hello?page=1 http://example.com
`,
	)
	assert := assert.New(t)
	assert.Equal(2, len(us))
	assert.Equal(us[0].String(), "http://example.com/doc/hello?page=1")
}
