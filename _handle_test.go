package crawler

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHandler(t *testing.T) {
	cw := newTestCrawler()
	handler := cw.newRespHandler()
	rs := []*Response{
		{
			Content:     []byte("<html>你好，世界</html>"),
			pview:       []byte("<html>你好，世界</html>"),
			ContentType: "text/html",
		}, {
			Content:     []byte("<html>你好，世界</html>"),
			pview:       []byte("<html>你好，世界</html>"),
			ContentType: "text/html; charset=utf-8",
		}, {
			Content:     []byte("<html><body></body></html>"),
			pview:       []byte("<html><body></body></html>"),
			ContentType: "text/html; charset=gbk",
		},
	}
	exp := []*Response{
		{
			Content:        []byte("<html>你好，世界</html>"),
			Charset:        "utf-8",
			CertainCharset: false,
		}, {
			Content:        []byte("<html>你好，世界</html>"),
			Charset:        "utf-8",
			CertainCharset: true,
		}, {
			Content:        []byte("<html><body></body></html>"),
			Charset:        "gbk",
			CertainCharset: true,
		},
	}

	for i, r := range rs {
		// r.BodyStatus = BodyStatusReady
		handler.handle(r)
		assert.True(t, bytes.Equal(exp[i].Content, r.Content))
		assert.Equal(t, exp[i].Charset, r.Charset)
		assert.Equal(t, exp[i].CertainCharset, r.CertainCharset)
	}
}
