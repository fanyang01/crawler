package crawler

import (
	"bytes"
	"io"
	"log"
	"net/url"
	"sync"

	"golang.org/x/net/html"
)

// Anchor represents a anchor found by crawler.
type Anchor struct {
	URL       *url.URL // parsed url
	Hyperlink bool     // is hyperlink?
	Text      []byte   // anchor text
	Depth     int      // length of path to find it
}

// Link is a collection of urls on a page.
type Link struct {
	Base    *url.URL
	Anchors []Anchor
}

type finder struct {
	In      <-chan *Response
	Out     chan *Link
	Done    chan struct{}
	WG      *sync.WaitGroup
	nworker int
}

func newFinder(nworker int) *finder {
	return &finder{
		Out:     make(chan *Link, nworker),
		nworker: nworker,
	}
}

func (f *finder) start() {
	var wg sync.WaitGroup
	wg.Add(f.nworker)
	for i := 0; i < f.nworker; i++ {
		go func() {
			f.work()
			wg.Done()
		}()
	}
	go func() {
		wg.Wait()
		close(f.Out)
		f.WG.Done()
	}()
}

func (f *finder) work() {
	for r := range f.In {
		if match := CT_HTML.match(r.ContentType); !match {
			continue
		}
		select {
		case f.Out <- findLink(r):
		case <-f.Done:
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
