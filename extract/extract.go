package crawler

import (
	"bytes"
	"io"
	"net/url"

	"github.com/fanyang01/crawler"

	"golang.org/x/net/html"
)

func ExtractLink(base *url.URL, reader io.Reader, ch chan<- *crawler.Link) error {
	z := html.NewTokenizer(reader)
LOOP:
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			if err := z.Err(); err != io.EOF {
				return err
			}
			break LOOP
		case html.StartTagToken, html.SelfClosingTagToken:
			linkFromToken(base, z, ch)
		}
	}
	return nil
}

func linkFromToken(base *url.URL, z *html.Tokenizer, ch chan<- *crawler.Link) {
	tn, hasAttr := z.TagName()
	if !hasAttr {
		return
	}
	if bytes.Equal(tn, []byte("base")) {
		if s, ok := lookupAttr(z, "href"); ok && s != "" {
			if u, err := base.Parse(s); err == nil {
				base = u
			}
		}
	}
	f := func(s string) {
		if u, err := base.Parse(s); err == nil {
			u.Fragment = ""
			ch <- &crawler.Link{
				URL: u,
			}
		}
	}
	for _, v := range []struct{ tag, attr string }{
		{"a", "href"},
		{"link", "href"},
		{"script", "src"},
		{"img", "src"},
		{"iframe", "src"},
	} {
		if bytes.Equal(tn, []byte(v.tag)) {
			if s, ok := lookupAttr(z, v.attr); ok && s != "" {
				f(s)
				return
			}
		}
	}
}

func lookupAttr(z *html.Tokenizer, attr string) (string, bool) {
	for {
		key, val, more := z.TagAttr()
		if string(key) == attr {
			return string(val), true
		}
		if !more {
			return "", false
		}
	}
}
