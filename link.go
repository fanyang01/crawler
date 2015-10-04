package crawler

import (
	"bytes"
	"io"
	"log"
	"net/url"
	"sync"
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

func (lp *linkParser) newDoc(resp *Response) *Doc {
	doc := &Doc{}
	doc.Loc = *resp.Locations
	doc.requestURL = resp.requestURL
	doc.SecondURL = resp.ContentLocation
	doc.Content = resp.Content
	doc.ContentType = resp.ContentType
	doc.Time = resp.Date
	doc.Expires = resp.Expires
	doc.SubURLsReady = make(chan struct{}, 1)
	doc.LastModified = resp.LastModified
	// HTTP prefer max-age than expires
	if resp.Cacheable && resp.MaxAge != 0 {
		doc.Expires = doc.Time.Add(resp.MaxAge)
	}
	return doc
}

type linkParser struct {
	option  *Option
	handler RHandler
	In      chan *Response
	Out     chan *Doc
	Done    chan struct{}
}

type RHandler interface {
	Handle(*Response, *Doc)
}

func newLinkParser(opt *Option, handler RHandler) *linkParser {
	lp := &linkParser{
		Out:     make(chan *Doc, opt.LinkParser.QLen),
		option:  opt,
		handler: handler,
	}
	return lp
}

func (lp *linkParser) Start() {
	n := lp.option.LinkParser.NWorker
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
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
		var doc *Doc
		if match := CT_HTML.match(r.ContentType); match {
			doc = lp.newDoc(r)
			go lp.findLink(doc)
		}
		// User-provided handler
		lp.handler.Handle(r, doc)
		r.CloseBody()
		if doc != nil {
			doc.Tree = nil
			// Fetch all unprocessed message
			for ok := true; ok; {
				_, ok = <-doc.SubURLsReady
			}
			select {
			case lp.Out <- doc:
			case <-lp.Done:
				return
			}
		} else {
			select {
			case <-lp.Done:
				return
			default:
			}
		}
	}
}

// ParseHTML parses the content into a HTML tree. It may result in many allocations.
func (doc *Doc) ParseHTML() (tree *html.Node, err error) {
	tree, err = html.Parse(bytes.NewReader(doc.Content))
	doc.Tree = tree
	return
}

func (lp *linkParser) findLink(doc *Doc) {
	z := html.NewTokenizer(bytes.NewReader(doc.Content))
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
						if u, err := doc.Loc.Parse(string(val)); err == nil {
							doc.SubURLs = append(doc.SubURLs, u)
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
	doc.SubURLsReady <- struct{}{}
	close(doc.SubURLsReady)
}
