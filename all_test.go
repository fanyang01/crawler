package crawler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type testHandler struct {
	OnceHandler
	text  chan string
	value chan []string
}

func (h testHandler) Recieve(r *Response) bool {
	h.text <- r.FindText("div.foo")
	h.value <- r.FindAttr("div#hello", "key")
	return true
}

func TestAll(t *testing.T) {
	assert := assert.New(t)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintln(w, `
<html>
<head></head>
<body>
<div class="foo">bar</div>
<div id="hello" key="value">Hello, world!</div>
</body>
</html>`)
	}))
	defer ts.Close()

	handler := &testHandler{
		text:  make(chan string, 2),
		value: make(chan []string, 2),
	}

	cw := NewCrawler(nil, nil, handler)
	assert.Nil(cw.Crawl(ts.URL))
	assert.Equal("bar", <-handler.text)
	vs := <-handler.value
	assert.Equal(1, len(vs))
	assert.Equal("value", vs[0])

	u, err := url.Parse(ts.URL)
	assert.Nil(err)
	uu, ok := cw.urlStore.Get(*u)
	assert.True(ok)
	assert.Equal(1, uu.Visited.Count)
	assert.True(uu.Visited.Time.After(time.Time{}))
	cw.Stop()
}
