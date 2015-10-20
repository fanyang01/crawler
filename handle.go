package crawler

import (
	"bytes"
	"errors"

	"github.com/PuerkitoBio/goquery"
)

var ErrNotHTML = errors.New("content type is not HTML")

type reciever struct {
	workerConn
	In      <-chan *Response
	Out     chan *Response
	ctrler Handler
}

func newRespHandler(nworker int, ctrler Handler) *reciever {
	this := &reciever{
		Out:     make(chan *Response, nworker),
		ctrler: ctrler,
	}
	this.nworker = nworker
	return this
}

func (rv *reciever) cleanup() { close(rv.Out) }

func (rv *reciever) work() {
	for r := range rv.In {
		follow := rv.ctrler.Handle(r)
		r.CloseBody()
		if !follow {
			r = nil // downstream should check nil
			continue
		}
		select {
		case rv.Out <- r:
		case <-rv.Done:
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
	if err = resp.ReadBody(MaxHTMLLen, true); err != nil {
		return
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
