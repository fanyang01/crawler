package crawler

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func newTestCrawler() *Crawler {
	opt := DefaultOption
	store := newMemStore()
	ctrl := DefaultController
	cw := &Crawler{
		opt:   opt,
		store: store,
		ctrl:  ctrl,
	}
	return cw
}

func TestHandler(t *testing.T) {
	cw := newTestCrawler()
	handler := cw.newRespHandler()
	rs := []*Response{
		{
			Content:     []byte("<html>你好，世界</html>"),
			ContentType: "text/html",
		}, {
			Content:     []byte("<html>你好，世界</html>"),
			ContentType: "text/html; charset=utf-8",
		}, {
			Content:     []byte("<html><body></body></html>"),
			ContentType: "text/html; charset=gbk",
		},
	}
	exp := []*Response{
		{
			Content:        []byte("<html>你好，世界</html>"),
			Charset:        "utf-8",
			CertainCharset: false,
			CharsetDecoded: false,
		}, {
			Content:        []byte("<html>你好，世界</html>"),
			Charset:        "utf-8",
			CertainCharset: true,
			CharsetDecoded: false,
		}, {
			Content:        []byte("<html><body></body></html>"),
			Charset:        "gbk",
			CertainCharset: true,
			CharsetDecoded: true,
		},
	}

	for i, r := range rs {
		r.BodyStatus = RespStatusReady
		handler.handle(r)
		assert.True(t, bytes.Equal(exp[i].Content, r.Content))
		assert.Equal(t, exp[i].Charset, r.Charset)
		assert.Equal(t, exp[i].CertainCharset, r.CertainCharset)
		assert.Equal(t, exp[i].CharsetDecoded, r.CharsetDecoded)
	}
}
