package crawler

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFetchParse(t *testing.T) {
	const page = `
<html>
<head>
<meta http-equiv="content-type" content="text/html;charset=GBK">
<meta charset="GBK">
<meta http-equiv="refresh" content="30; URL=1.html">
</head>
<body>
</body>
</html>
`
	checkErr := func(err error) {
		if err != nil {
			t.Log(err)
			t.FailNow()
		}
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Content-Location", "index.html")
		fmt.Fprint(w, page)
	}))
	defer ts.Close()

	url := ts.URL + "/hello/"
	re, err := http.NewRequest("GET", url, nil)
	checkErr(err)
	req := &Request{
		Request: re,
	}
	checkErr(err)
	resp, err := DefaultClient.Do(req)
	checkErr(err)

	assert := assert.New(t)

	resp.scanLocation()
	resp.detectContentType()

	preview, err := resp.preview(1024)
	assert.Nil(err)

	resp.scanHTMLMeta(preview)

	assert.Equal(url, resp.NewURL.String())
	assert.Equal(`text/html; charset=gbk`, resp.ContentType)
	assert.Equal("gbk", resp.Charset)
	assert.Equal(url+"1.html", resp.Refresh.URL.String())
	assert.Equal(url+"index.html", resp.ContentLocation.String())
	assert.Equal(30, resp.Refresh.Seconds)
}

func TestConvToUTF8(t *testing.T) {
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
	exp := []struct {
		Charset        string
		CertainCharset bool
		Content        []byte
	}{
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
	assert := assert.New(t)

	for i, r := range rs {
		u, _ := url.Parse(fmt.Sprintf("/hello/%d", i))
		r.URL = u
		r.NewURL = u
		preview, err := r.preview(1024)
		assert.NoError(err)
		r.convToUTF8(preview, func(_ *url.URL) string { return "utf-8" })
		assert.Equal(exp[i].Charset, r.Charset)
		assert.Equal(exp[i].CertainCharset, r.CertainCharset)
		b, err := ioutil.ReadAll(r.Body)
		assert.NoError(err)
		assert.Equal(exp[i].Content, b)
	}
}
