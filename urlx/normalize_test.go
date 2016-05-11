package urlx

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeURL(t *testing.T) {
	data := [][2]string{
		{"http://example.com", "http://example.com"},
		{"hTTp://eXAMPle.com", "http://example.com"},
		{"http://example.com:80", "http://example.com"},
		{"https://example.com:443", "https://example.com"},
		{"http://中文.com", "http://xn--fiq228c.com"},
		{"http://xn--FIQ228c.com", "http://xn--fiq228c.com"},
	}
	assert := assert.New(t)
	for _, v := range data {
		u, err := Parse(v[0], Normalize)
		assert.NoError(err)
		assert.NotNil(u)
		assert.Equal(v[1], u.String())
	}
	invalid := []string{
		"http://example.com/\xB4\xBA\xBD\xDA",
		"http://example.com/?hello=\xB4\xBA\xBD\xDA",
	}
	for _, v := range invalid {
		u, err := Parse(v, Normalize)
		fmt.Printf("%#v\n", u)
		fmt.Println(err)
		assert.Error(err)
	}
}
