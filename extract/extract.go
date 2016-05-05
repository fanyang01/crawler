// Package extract parses, extracts and filters URLs.
package extract

import (
	"io"
	"net/url"

	"github.com/fanyang01/crawler"

	"golang.org/x/net/html"
)

// Extractor extracts and filters URLs.
type Extractor struct {
	Normalize   func(*url.URL) error
	Matcher     Matcher
	Destination []struct{ Tag, Attr string }
	MaxDepth    int
	SpanHosts   bool
	SameOrigin  bool
}

// Extract parses the HTML document, extracts URLs and filters them using
// the matcher.
func (e *Extractor) Extract(
	r *crawler.Response, body io.Reader, ch chan<- *crawler.Link,
) error {
	if e.MaxDepth > 0 {
		if depth, err := r.Context().Depth(); err != nil {
			return err
		} else if depth >= e.MaxDepth {
			return nil
		}
	}
	chURL := make(chan *url.URL, 32)
	chErr := make(chan error, 1)
	go e.tokenLoop(r, body, chURL, chErr)

	scheme, host := r.NewURL.Scheme, r.NewURL.Host
	for u := range chURL {
		if e.SameOrigin && u.Scheme != scheme {
			continue
		} else if !e.SpanHosts && u.Host != host {
			continue
		} else if !e.Matcher.Match(u) {
			continue
		}
		ch <- &crawler.Link{URL: u}
	}
	return <-chErr
}

func (e *Extractor) tokenLoop(
	r *crawler.Response, body io.Reader, ch chan<- *url.URL, chErr chan<- error,
) {
	defer close(chErr)
	defer close(ch)

	z := html.NewTokenizer(body)
	base := *r.NewURL
	normalize := e.Normalize
	dest := e.Destination
	if normalize == nil {
		normalize = crawler.NormalizeURL
	}
	if len(dest) == 0 {
		dest = []struct{ Tag, Attr string }{{"a", "href"}}
	}

	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			if err := z.Err(); err != io.EOF {
				chErr <- err
			}
			return
		case html.StartTagToken, html.SelfClosingTagToken:
			token := z.Token()
			if len(token.Attr) == 0 {
				continue
			}
			var (
				v    string
				u    *url.URL
				ok   bool
				err  error
				name = string(token.Data)
			)
			for _, d := range dest {
				if name != d.Tag {
					continue
				} else if v, ok = get(&token, d.Attr); !ok || v == "" {
					continue
				} else if u, err = crawler.ParseURLFrom(&base, v); err != nil {
					continue
				}
				if name == "base" {
					base = *u
				}
				if err = normalize(u); err != nil {
					continue
				}
				ch <- u
			}
		}
	}
}

func get(t *html.Token, attr string) (v string, ok bool) {
	for _, a := range t.Attr {
		if a.Key == attr {
			return a.Val, true
		}
	}
	return "", false
}
