package crawler

import (
	"bytes"
	"log"
	"net/url"
	"time"

	"golang.org/x/net/html"
)

type Doc struct {
	URL
	requestURL   *url.URL
	SecondURL    *url.URL
	SubURLs      []*url.URL
	SubURLsReady chan struct{}
	Tree         *html.Node
	TreeReady    chan struct{}
	Content      []byte
	ContentType  string
	Time         time.Time
	Expires      time.Time
}

func newDoc(resp *Response) *Doc {
	doc := &Doc{
		requestURL:   resp.requestURL,
		SecondURL:    resp.ContentLocation,
		Content:      resp.Content,
		ContentType:  resp.ContentType,
		Time:         resp.Date,
		Expires:      resp.Expires,
		TreeReady:    make(chan struct{}, 1),
		SubURLsReady: make(chan struct{}, 1),
	}
	doc.Loc = *resp.Locations
	doc.LastModified = resp.LastModified
	// HTTP prefer max-age than expires
	if resp.Cacheable && resp.MaxAge != 0 {
		doc.Expires = doc.Time.Add(resp.MaxAge)
	}
	return doc
}

type linkParser struct {
	In      chan *Response
	Out     chan *Doc
	option  *Option
	workers chan struct{}
}

type RHandler interface {
	Handle(*Response, *Doc)
}

func newLinkParser(opt *Option) *linkParser {
	return &linkParser{
		Out:     make(chan *Doc, opt.LinkParser.OutQueueLen),
		option:  opt,
		workers: make(chan struct{}, opt.LinkParser.NumOfWorkers),
	}
}

func (lp *linkParser) Start(handler RHandler) {
	for i := 0; i < lp.option.LinkParser.NumOfWorkers; i++ {
		lp.workers <- struct{}{}
	}
	go func() {
		for resp := range lp.In {
			<-lp.workers
			go func(r *Response) {
				defer func() { lp.workers <- struct{}{} }()
				var doc *Doc
				if match := CT_HTML.match(r.ContentType); match {
					doc = newDoc(r)
					go extractLink(doc)
				}
				// User-provided handler
				handler.Handle(r, doc)
				r.CloseBody()
				if doc != nil {
					doc.Tree = nil
					// Fetch all unprocessed message
					for ok := true; ok; {
						_, ok = <-doc.TreeReady
					}
					for ok := true; ok; {
						_, ok = <-doc.SubURLsReady
					}
					lp.Out <- doc
				}
			}(resp)
		}
		close(lp.Out)
	}()
}

func extractLink(doc *Doc) {
	tree, err := html.Parse(bytes.NewReader(doc.Content))
	if err != nil {
		log.Printf("extractLink: %v\n", err)
		return
	}
	doc.Tree = tree
	doc.TreeReady <- struct{}{}
	close(doc.TreeReady)

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
	doc.SubURLsReady <- struct{}{}
	close(doc.SubURLsReady)
}
