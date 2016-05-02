package crawler

import (
	"io"

	"golang.org/x/net/html"
)

const LinkPerPage = 32

func (f *handler) findLink(resp *Response, depth int, reader io.Reader) {
	z := html.NewTokenizer(reader)
	ch := make(chan *Link, LinkPerPage)
	done := make(chan struct{})
	// go f.consume(resp, ch, done)

LOOP:
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			if z.Err() != io.EOF {
				log.Errorf("find link: %v", z.Err())
			}
			break LOOP
		case html.StartTagToken:
			tn, hasAttr := z.TagName()
			if hasAttr && len(tn) == 1 && tn[0] == 'a' {
				for {
					key, val, more := z.TagAttr()
					if string(key) == "href" {
						if u, err := resp.NewURL.Parse(string(val)); err == nil {
							u.Fragment = ""
							ch <- &Link{
								URL:       u,
								Hyperlink: u.Host != resp.NewURL.Host,
								Depth:     depth + 1,
								// TODO: anchor text
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
	close(ch)
	<-done
}
