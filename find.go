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
