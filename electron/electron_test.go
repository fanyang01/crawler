// +build integration

package electron

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"testing"

	"golang.org/x/net/context"

	"github.com/fanyang01/crawler"
	"github.com/stretchr/testify/assert"
)

func assertResponse(t *testing.T, oldURL, newURL string, r *crawler.Response) {
	assert := assert.New(t)
	assert.Equal(200, r.StatusCode)
	assert.Equal("200 OK", r.Status)
	assert.Equal(oldURL, r.URL.String())
	assert.Equal(newURL, r.NewURL.String())
	assert.Equal("GET", r.Request.Method)
	assert.Equal(newURL, r.Request.URL.String())
	assert.Contains(r.Header.Get("Content-Type"), "text/html")
	assert.Contains(r.Header, "Date")
	assert.True(bytes.Contains(r.Content, []byte("Standard library")))
	assert.NotNil(r.Body)
}

func TestElectronNats(t *testing.T) {
	conn, err := NewElectronConn(nil)
	if err != nil {
		t.Fatal(err)
	}
	var host string
	if host = os.Getenv("GODOC_SERVER_ADDR"); host == "" {
		host = "http://localhost:6060"
	}
	URL := fmt.Sprintf("%s/pkg", host)
	rq, err := http.NewRequest("GET", URL, nil)
	req := &crawler.Request{
		Request: rq,
		Context: context.TODO(),
	}
	resp, err := conn.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	assertResponse(t, URL, URL+"/", resp)
}
