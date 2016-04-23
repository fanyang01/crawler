package crawler

import (
	"io"

	"golang.org/x/net/html"
)

const LinkPerPage = 32

func (f *handler) find(r *Response, reader io.Reader, done chan<- struct{}) {
	defer func() { done <- struct{}{} }()

	depth, err := f.cw.store.GetDepth(r.URL)
	if err != nil {
		log.Error(err)
		return
	}
	// Treat the new url as one found under the original url
	original := r.URL.String()
	if r.NewURL.String() != original {
		f.filter(r, &Link{
			URL:   r.NewURL,
			Depth: depth + 1,
		})
	}
	if refresh := r.Refresh.URL; refresh != nil && refresh.String() != original {
		f.filter(r, &Link{
			URL:   r.Refresh.URL,
			Depth: depth + 1,
		})
	}
	f.findLink(r, depth, reader)
}

func (f *handler) filter(resp *Response, link *Link) {
	if f.cw.ctrl.Accept(resp, link) {
		// only handle new link
		if ok, err := f.cw.store.Exist(link.URL); err != nil {
			log.Error(err)
			return
		} else if ok {
			return
		}
		if ok, err := f.cw.store.PutNX(&URL{
			Loc:   *link.URL,
			Depth: link.Depth,
		}); err != nil {
			log.Error(err)
			return
		} else if ok {
			resp.links = append(resp.links, link)
		}
	}
}

func (f *handler) findLink(resp *Response, depth int, reader io.Reader) {
	z := html.NewTokenizer(reader)
	ch := make(chan *Link, LinkPerPage)
	done := make(chan struct{})
	go f.consume(resp, ch, done)

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

func (f *handler) consume(resp *Response, ch <-chan *Link, done chan<- struct{}) {
	for link := range ch {
		f.filter(resp, link)
	}
	done <- struct{}{}
}
