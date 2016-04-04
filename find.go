package crawler

import (
	"bytes"
	"io"

	"github.com/Sirupsen/logrus"

	"golang.org/x/net/html"
)

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
		select {
		case f.Out <- findLink(f.cw, r):
		case <-f.quit:
			return
		}
	}
}

func findLink(cw *Crawler, resp *Response) *Response {
	z := html.NewTokenizer(bytes.NewReader(resp.Content))
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
							resp.links = append(resp.links, &Link{
								URL:       u,
								Hyperlink: u.Host != resp.NewURL.Host,
								// TODO: anchor text
							})
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
	return resp
}
