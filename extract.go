package crawler

import (
	"io"
	"net/url"

	"golang.org/x/net/html"
)

const LinkPerPage = 32

func ExtractHref(base *url.URL, reader io.Reader, ch chan<- *Link) error {
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
		case html.StartTagToken:
			tn, hasAttr := z.TagName()
			if hasAttr && len(tn) == 1 && tn[0] == 'a' {
				for {
					key, val, more := z.TagAttr()
					if string(key) == "href" {
						if u, err := base.Parse(string(val)); err == nil {
							u.Fragment = ""
							ch <- &Link{
								URL: u,
							}
						}
						break
					}
					if !more {
						break
					}
				}
			}
		}
	}
	return nil
}

func ExtractLink(base *url.URL, reader io.Reader, ch chan<- *Link) error {
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

func linkFromToken(base *url.URL, z *html.Tokenizer, ch chan<- *Link) {
	f := func(s string) {
		if u, err := base.Parse(s); err == nil {
			u.Fragment = ""
			ch <- &Link{
				URL: u,
			}
		}
	}
	tn, hasAttr := z.TagName()
	if !hasAttr {
		return
	}
	for _, v := range []struct{ tag, attr string }{
		{"a", "href"},
		{"link", "href"},
		{"script", "src"},
		{"img", "src"},
		{"iframe", "src"},
	} {
		if string(tn) == v.tag {
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
