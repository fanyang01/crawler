package crawler

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

type handleTestCtrl struct {
	OnceController
	content [][]byte
}

func (c *handleTestCtrl) Handle(r *Response, _ chan<- *Link) {
	b, _ := ioutil.ReadAll(r.Body)
	c.content = append(c.content, b)
}

func TestHandler(t *testing.T) {
	ctrl := &handleTestCtrl{}
	cw := NewCrawler(&Config{Controller: ctrl})
	handler := cw.handler
	rs := []*Response{
		{
			Body:        strings.NewReader("<html>你好，世界</html>"),
			ContentType: "text/html",
		}, {
			Body:        strings.NewReader("<html>你好，世界</html>"),
			ContentType: "text/html; charset=utf-8",
		}, {
			Body:        strings.NewReader("<html><body></body></html>"),
			ContentType: "text/html; charset=gbk",
		},
	}
	exp := []*Response{
		{
			Charset:        "utf-8",
			CertainCharset: false,
		}, {
			Charset:        "utf-8",
			CertainCharset: true,
		}, {
			Charset:        "gbk",
			CertainCharset: true,
		},
	}

	for i, r := range rs {
		u, _ := url.Parse(fmt.Sprintf("/hello/%d", i))
		r.URL = u
		r.NewURL = u
		r.ctx = newContext(cw, u)
		cw.store.PutNX(&URL{Loc: *u})
		handler.handle(r)

		assert.Equal(t, exp[i].Charset, r.Charset)
		assert.Equal(t, exp[i].CertainCharset, r.CertainCharset)
	}
}
