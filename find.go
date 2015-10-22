package crawler

import (
	"bytes"
	"io"
	"net/url"

	log "github.com/Sirupsen/logrus"
	"golang.org/x/net/html"
)

// Anchor represents a anchor found by crawler.
type Anchor struct {
	URL       *url.URL // parsed url
	Hyperlink bool     // is hyperlink?
	Text      []byte   // anchor text
	Depth     int      // length of path to find it
	ok        bool
}

// Link is a collection of urls on a page.
type Link struct {
	Base    *url.URL
	Anchors []*Anchor
}

type finder struct {
	workerConn
	In        <-chan *Response
	Out       chan *Link
	statistic *Statistic
}

func (cw *Crawler) newFinder() *finder {
	nworker := cw.opt.NWorker.Finder
	this := &finder{
		Out:       make(chan *Link, nworker),
		statistic: &cw.statistic,
	}
	this.nworker = nworker
	this.wg = &cw.wg
	this.quit = cw.quit
	return this
}

func (f *finder) cleanup() { close(f.Out) }

func (f *finder) work() {
	for r := range f.In {
		select {
		case f.Out <- findLink(r):
		case <-f.quit:
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
				log.Errorf("find link: %v", z.Err())
			}
			break LOOP
		case html.StartTagToken:
			tn, hasAttr := z.TagName()
			if hasAttr && len(tn) == 1 && tn[0] == 'a' {
				for {
					key, val, more := z.TagAttr()
					if string(key) == "href" {
						if u, err := resp.Locations.Parse(string(val)); err == nil {
							link.Anchors = append(link.Anchors, &Anchor{
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
