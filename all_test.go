package crawler

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type testController struct {
	OnceController
	content chan []byte
}

func (t testController) Handle(r *Response, _ chan<- *Link) {
	b, _ := ioutil.ReadAll(r.Body)
	t.content <- b
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

	ctrl := &testController{
		content: make(chan []byte, 2),
	}

	cw := NewCrawler(&Config{Controller: ctrl})
	assert.Nil(cw.Crawl(ts.URL))
	cw.Wait()
	content := string(<-ctrl.content)
	assert.Contains(content, "bar")
	assert.Contains(content, "Hello, world!")

	u, err := url.Parse(ts.URL)
	assert.Nil(err)
	uu, _ := cw.store.Get(u)
	assert.Equal(1, uu.VisitCount)
	assert.True(uu.Last.After(time.Time{}))
}
