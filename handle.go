package crawler

import (
	"bytes"
	"errors"
	"net/url"

	"github.com/PuerkitoBio/goquery"
)

var ErrNotHTML = errors.New("content type is not HTML")

type handler struct {
	workerConn
	In      <-chan *Response
	Out     chan *Response
	DoneOut chan *url.URL
	cw      *Crawler
}

func (cw *Crawler) newRespHandler() *handler {
	nworker := cw.opt.NWorker.Handler
	this := &handler{
		Out: make(chan *Response, nworker),
		cw:  cw,
	}
	cw.initWorker(this, nworker)
	return this
}

func (rv *handler) cleanup() { close(rv.Out) }

func (rv *handler) work() {
	for r := range rv.In {
		follow := rv.cw.ctl.Handle(r)
		r.CloseBody()
		if !follow || !CT_HTML.match(r.ContentType) {
			rv.DoneOut <- r.NewURL
			r.NewURL = nil
			r = nil
			continue
		}
		select {
		case rv.Out <- r:
		case <-rv.quit:
			return
		}
	}
}

// Document parses content of response into HTML document if it has not been
// parsed. Unread response will be read.
func (resp *Response) Document() (doc *goquery.Document, err error) {
	if resp.document != nil {
		return resp.document, nil
	}
	if !CT_HTML.match(resp.ContentType) {
		return nil, ErrNotHTML
	}
	if doc, err = goquery.NewDocumentFromReader(
		bytes.NewReader(resp.Content)); err != nil {
		return
	}
	resp.document = doc
	return
}

// FindText gets text content of all elements matching selector.
func (resp *Response) FindText(selector string) string {
	if docErr(resp) {
		return ""
	}
	return resp.document.Find(selector).Text()
}

// FindAttr gets all values of attribute in elements matching selector.
func (resp *Response) FindAttr(selector, attr string) (values []string) {
	if docErr(resp) {
		return
	}
	resp.document.Find(selector).Each(
		func(_ int, s *goquery.Selection) {
			if v, ok := s.Attr(attr); ok {
				values = append(values, v)
			}
		})
	return
}

func docErr(resp *Response) bool {
	if _, err := resp.Document(); err != nil {
		return true
	}
	return false
}
