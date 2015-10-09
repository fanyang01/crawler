package crawler

import (
	"bytes"
	"io"
	"log"
	"net/url"
	"sync"

	"golang.org/x/net/html"
)

type Anchor struct {
	URL       *url.URL
	Hyperlink bool
	Text      []byte
}

type Link struct {
	Base    *url.URL
	Anchors []Anchor
}

type linkParser struct {
	In      <-chan *Response
	Out     chan *Link
	Done    chan struct{}
	nworker int
}

func newLinkParser(nworker int, in <-chan *Response, done chan struct{}) *linkParser {
	return &linkParser{
		Out:     make(chan *Link, nworker),
		In:      in,
		Done:    done,
		nworker: nworker,
	}
}

func (lp *linkParser) start() {
	var wg sync.WaitGroup
	wg.Add(lp.nworker)
	for i := 0; i < lp.nworker; i++ {
		go func() {
			lp.work()
			wg.Done()
		}()
	}
	go func() {
		wg.Wait()
		close(lp.Out)
	}()
}

func (lp *linkParser) work() {
	for r := range lp.In {
		if r == nil {
			continue
		}
		if match := CT_HTML.match(r.ContentType); !match {
			continue
		}
		select {
		case lp.Out <- findLink(r):
		case <-lp.Done:
			return
		}
	}
}

func findLink(resp *Response) *Link {
	link := &Link{
		Base: resp.Locations,
	}
	z := html.NewTokenizer(bytes.NewReader(resp.Content))
LOOP:
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			if z.Err() != io.EOF {
				log.Printf("find link: %v\n", z.Err())
			}
			break LOOP
		case html.StartTagToken:
			tn, hasAttr := z.TagName()
			if hasAttr && len(tn) == 1 && tn[0] == 'a' {
				for {
					key, val, more := z.TagAttr()
					if string(key) == "href" {
						if u, err := resp.Locations.Parse(string(val)); err == nil {
							link.Anchors = append(link.Anchors, Anchor{
								URL:       u,
								Hyperlink: u.Host != link.Base.Host,
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
	return link
}
