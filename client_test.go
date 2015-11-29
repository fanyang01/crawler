package crawler

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClient(t *testing.T) {
	checkErr := func(err error) {
		if err != nil {
			t.Log(err)
			t.FailNow()
		}
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "foobar")
	}))
	defer ts.Close()

	re, err := http.NewRequest("GET", ts.URL, nil)
	checkErr(err)
	req := &Request{
		Request: re,
	}
	checkErr(err)
	resp, err := DefaultClient.Do(req)
	checkErr(err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, ts.URL, resp.NewURL.String())
	assert.True(t, CT_PLAIN.match(resp.ContentType))
	assert.Nil(t, resp.ReadBody(1<<10))
	assert.True(t, resp.Closed)
	assert.True(t, bytes.Equal(resp.Content, []byte("foobar\n")))
}
