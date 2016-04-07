package crawler

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
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

	cw := newTestCrawler()
	f := cw.newFetcher()

	f.parse(resp)

	assert := assert.New(t)

	assert.Equal(url, resp.NewURL.String())
	assert.Equal(`text/html; charset=gbk`, resp.ContentType)
	assert.Equal("gbk", resp.Charset)
	assert.Equal(url+"1.html", resp.Refresh.URL.String())
	assert.Equal(30, resp.Refresh.Seconds)
	assert.Nil(resp.ReadBody(1 << 10))
	assert.Equal(RespStatusReady, resp.BodyStatus)
	assert.True(bytes.Equal(resp.Content, []byte(page)))
}
