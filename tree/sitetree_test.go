package crawler

import (
	"log"
	"net/url"
	"testing"
)

func TestSiteTree(t *testing.T) {
	testURLs := []string{
		"http://example.com/search?q=hello",
		"http://example.com/movies/id/123456",
		"http://example.com/books/id/123456",
		"http://example.com/movies",
		"http://example.com/",
	}

	site := NewSiteTree("http", "example.com")

	for _, ul := range testURLs {
		u, _ := url.Parse(ul)
		if _, ok := site.Insert(*u); !ok {
			log.Println(ul)
			t.Fail()
		}
	}

	for _, ul := range testURLs {
		u, _ := url.Parse(ul)
		if ok := site.Contain(*u); !ok {
			log.Println(ul)
			t.Fail()
		}
	}
}
