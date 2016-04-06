package crawler

import (
	"bytes"
	"io"

	"github.com/Sirupsen/logrus"

	"golang.org/x/net/html"
)

const LinkPerPage = 32

type finder struct {
	workerConn
	cw  *Crawler
	In  <-chan *Response
	Out chan *Response
}

func (cw *Crawler) newFinder() *finder {
	nworker := cw.opt.NWorker.Finder
	this := &finder{
		cw:  cw,
		Out: make(chan *Response, nworker),
	}
	cw.initWorker(this, nworker)
	return this
}

func (f *finder) cleanup() { close(f.Out) }

func (f *finder) work() {
	for r := range f.In {
		// Treat the new url as one found under the original url
		if r.NewURL.String() != r.RequestURL.String() {
			r.links = append(r.links, &Link{
				URL: r.NewURL,
			})
		}
		if r.follow {
			f.findLink(r)
		}
		select {
		case f.Out <- r:
		case <-f.quit:
			return
		}
	}
}

func (f *finder) findLink(resp *Response) {
	depth := f.cw.store.GetDepth(resp.RequestURL)
	z := html.NewTokenizer(bytes.NewReader(resp.Content))
	ch := make(chan *Link, LinkPerPage)
	done := make(chan struct{})
	go f.filter(resp, ch, done)

LOOP:
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			if z.Err() != io.EOF {
				logrus.Errorf("find link: %v", z.Err())
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

func (f *finder) filter(resp *Response, ch <-chan *Link, done chan<- struct{}) {
	for link := range ch {
		if f.cw.ctrl.Accept(link) {
			// only handle new link
			if f.cw.store.Exist(link.URL) {
				continue
			}
			if f.cw.store.PutNX(&URL{
				Loc:   *link.URL,
				Depth: link.Depth,
			}) {
				resp.links = append(resp.links, link)
			}
		}
	}
	done <- struct{}{}
}
