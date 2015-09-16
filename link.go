package crawler

import (
	"bytes"
	"log"

	"golang.org/x/net/html"
)

var (
	LinkBufSize = 64
)

type linkParser struct {
	In      chan *Doc
	Out     chan *Doc
	option  *Option
	workers chan struct{}
}

func newLinkParser(opt *Option) *linkParser {
	return &linkParser{
		Out:     make(chan *Doc, opt.LinkParser.OutQueueLen),
		option:  opt,
		workers: make(chan struct{}, opt.LinkParser.NumOfWorkers),
	}
}

func (lp *linkParser) Start() {
	for i := 0; i < lp.option.LinkParser.NumOfWorkers; i++ {
		lp.workers <- struct{}{}
	}
	go func() {
		for doc := range lp.In {
			<-lp.workers
			go func(d *Doc) {
				extractLink(d)
				lp.Out <- d
				lp.workers <- struct{}{}
			}(doc)
		}
		close(lp.Out)
	}()
}

func extractLink(doc *Doc) {
	if match := CT_HTML.match(doc.ContentType); !match {
		return
	}
	tree, err := html.Parse(bytes.NewReader(doc.Content))
	if err != nil {
		log.Printf("extractLink: %v\n", err)
		return
	}

	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, a := range n.Attr {
				if a.Key == "href" {
					if url, err := doc.Loc.Parse(a.Val); err == nil {
						doc.SubURLs = append(doc.SubURLs, url)
					} else {
						log.Printf("extractLink: %v\n", err)
					}
					break
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(tree)
}
