package crawler

import (
	"bytes"
	"io"
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

func (cw *Crawler) newDoc(resp *Response) *Doc {
	doc := cw.docPool.Get().(*Doc)
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

func (cw *Crawler) freeDoc(doc *Doc) {
	// enable GC
	doc.requestURL = nil
	doc.SecondURL = nil
	doc.SubURLs = nil
	doc.Tree = nil
	doc.Content = nil
	doc.SubURLsReady = nil
	cw.docPool.Put(doc)
}

func (cw *Crawler) newurl() *url.URL {
	return cw.urlPool.Get().(*url.URL)
}

func (cw *Crawler) freeurl(url *url.URL) {
	cw.urlPool.Put(url)
}

type linkParser struct {
	In      chan *Response
	Out     chan *Doc
	option  *Option
	workers chan struct{}
	cw      *Crawler
}

type RHandler interface {
	Handle(*Response, *Doc)
}

func newLinkParser(cw *Crawler, opt *Option) *linkParser {
	return &linkParser{
		Out:     make(chan *Doc, opt.LinkParser.OutQueueLen),
		option:  opt,
		workers: make(chan struct{}, opt.LinkParser.NumOfWorkers),
		cw:      cw,
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
					doc = lp.cw.newDoc(r)
					go lp.findLink(doc)
				}
				// User-provided handler
				handler.Handle(r, doc)
				r.CloseBody()
				if doc != nil {
					doc.Tree = nil
					// Fetch all unprocessed message
					for ok := true; ok; {
						_, ok = <-doc.SubURLsReady
					}
					lp.Out <- doc
				}
				lp.cw.freeResp(r)
			}(resp)
		}
		close(lp.Out)
	}()
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
						u := lp.cw.newurl()
						if err := ParseURL(&doc.Loc, string(val), u); err == nil {
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
