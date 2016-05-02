package crawler

import (
	"fmt"
	"io/ioutil"
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
	assert := assert.New(t)
	resp, err := DefaultClient.Do(req)
	checkErr(err)
	assert.Equal(200, resp.StatusCode)
	assert.Equal(ts.URL, resp.URL.String())
	assert.NotNil(resp.Body)
	assert.Equal(BodyStatusHeadOnly, resp.BodyStatus)
	b, err := ioutil.ReadAll(resp.Body)
	assert.Equal([]byte("foobar\n"), b)
	assert.Equal(BodyStatusEOF, resp.BodyStatus)
}
