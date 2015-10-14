package crawler

import (
	"bytes"
	"errors"
	"sync"

	"github.com/PuerkitoBio/goquery"
)

var ErrNotHTML = errors.New("content type is not HTML")

type respHandler struct {
	In      <-chan *Response
	Out     chan *Response
	Req     chan<- *HandlerQuery
	Done    chan struct{}
	nworker int
}

func newRespHandler(nworker int, in <-chan *Response, done chan struct{},
	ch chan<- *HandlerQuery) *respHandler {
	return &respHandler{
		In:   in,
		Req:  ch,
		Out:  make(chan *Response, nworker),
		Done: done,
	}
}

func (h *respHandler) start() {
	var wg sync.WaitGroup
	wg.Add(h.nworker)
	for i := 0; i < h.nworker; i++ {
		go func() {
			h.work()
			wg.Done()
		}()
	}
	go func() {
		wg.Wait()
		close(h.Out)
	}()
}

func (h *respHandler) work() {
	for r := range h.In {
		q := &HandlerQuery{
			URL:   r.Locations,
			Reply: make(chan Handler),
		}
		h.Req <- q
		H := <-q.Reply
		follow := H.Handle(r)
		r.CloseBody()
		if !follow {
			r = nil // downstream should check nil
			continue
		}
		select {
		case h.Out <- r:
		case <-h.Done:
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
