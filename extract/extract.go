// Package extract parses, extracts and filters URLs.
package extract

import (
	"io"
	"net/url"

	"github.com/fanyang01/crawler"
	"github.com/fanyang01/crawler/urlx"

	"golang.org/x/net/html"
)

// Extractor extracts and filters URLs.
type Extractor struct {
	Normalize  func(*url.URL) error
	Matcher    Matcher
	MaxDepth   int
	SpanHosts  bool
	SameOrigin bool
	Pos        []struct{ Tag, Attr string }
	Redirect   bool
	SniffFlags int
}

// Extract parses the HTML document, extracts URLs and filters them using
// the matcher.
func (e *Extractor) Extract(
	r *crawler.Response, body io.Reader, ch chan<- *url.URL,
) error {
	if e.MaxDepth > 0 {
		if r.Context().Depth() >= e.MaxDepth {
			return nil
		}
	}
	chURL := make(chan *url.URL, 32)
	if e.Redirect {
		newurl := *r.NewURL
		chURL <- &newurl
		if r.Refresh.URL != nil {
			refresh := *r.Refresh.URL
			chURL <- &refresh
		}
	}
	chErr := make(chan error, 1)
	go e.tokenLoop(r, body, chURL, chErr)

	scheme, host := r.URL.Scheme, r.URL.Host
	for u := range chURL {
		if e.SameOrigin && u.Scheme != scheme {
			continue
		} else if !e.SpanHosts && u.Host != host {
			continue
		} else if !e.Matcher.Match(u) {
			continue
		}
		ch <- u
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
	dest := e.Pos
	if normalize == nil {
		normalize = urlx.Normalize
	}
	if len(dest) == 0 {
		dest = []struct{ Tag, Attr string }{{"a", "href"}}
	}

	var prev html.Token
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
			prev = token
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
				} else if u, err = urlx.ParseRef(
					&base, v,
				); err != nil {
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
		case html.TextToken:
			token := z.Token()
			var urls []*url.URL
			switch {
			case e.SniffFlags&SniffWindowLocation != 0:
				if prev.Type == html.StartTagToken && prev.Data == "script" {
					urls = windowLocation(&base, token.Data)
				}
			case e.SniffFlags&SniffAbsoluteURLs != 0:
				urls = absoluteURLs(&base, token.Data)
			}
			for _, u := range urls {
				if err := normalize(u); err != nil {
					continue
				}
				ch <- u
			}
			prev = token
		default:
			prev = html.Token{}
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
