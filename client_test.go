package crawler

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fanyang01/crawler/cache"
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

func TestClientCache(t *testing.T) {
	assert := assert.New(t)
	checkErr := func(err error) {
		if err != nil {
			t.Error(err)
			t.FailNow()
		}
	}
	client := &StdClient{
		Client: &http.Client{},
		Cache:  cache.NewPool(1 << 20),
	}

	magic := "00000"
	count := 0
	notModified := 0
	mux := http.NewServeMux()
	mux.Handle("/normal", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		if etag := r.Header.Get("If-None-Match"); etag == magic {
			notModified++
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Cache-Control", "max-age=1")
		w.Header().Set("ETag", magic)
		fmt.Fprint(w, magic)
	}))
	mux.Handle("/revalidate", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		if etag := r.Header.Get("If-None-Match"); etag == magic {
			notModified++
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("ETag", magic)
		fmt.Fprint(w, magic)
	}))
	ts := httptest.NewServer(mux)

	var cnt int
	var notMod int
	f := func(u string, content string, server bool, nomod bool) {
		hreq, err := http.NewRequest("GET", ts.URL+u, nil)
		checkErr(err)
		req := &Request{
			Request: hreq,
		}
		r, err := client.Do(req)
		checkErr(err)
		b, _ := ioutil.ReadAll(r.Body)
		assert.Equal(content, string(b))
		if server {
			cnt++
			if nomod {
				notMod++
			}
		}
		assert.Equal(count, cnt)
		assert.Equal(notModified, notMod)
	}

	f("/normal", magic, true, false)
	f("/normal", magic, false, false) // should access cache rather than server
	f("/revalidate", magic, true, false)
	f("/revalidate", magic, true, true)
	time.Sleep(500 * time.Millisecond)
	magic = "22222"
	f("/normal", "00000", false, false)
	f("/revalidate", magic, true, false)
	time.Sleep(500 * time.Millisecond)
	f("/normal", magic, true, false)
	f("/normal", magic, false, false)
	f("/revalidate", magic, true, true)
	f("/revalidate", magic, true, true)
}
