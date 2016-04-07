package crawler

import (
	"bytes"
	"errors"
	"net/url"

	"golang.org/x/net/html/charset"

	"github.com/PuerkitoBio/goquery"
	"github.com/fanyang01/crawler/util"
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

func (h *handler) cleanup() { close(h.Out) }

func (h *handler) work() {
	for r := range h.In {
		h.handle(r)
		select {
		case h.Out <- r:
		case <-h.quit:
			return
		}
	}
}

func (h *handler) handle(r *Response) {
	if CT_HTML.match(r.ContentType) {
		e, name, certain := charset.DetermineEncoding(r.Content, r.ContentType)
		// according to charset package source, default unknown charset is windows-1252.
		if !certain && name == "windows-1252" {
			e = h.cw.opt.UnknownEncoding
			name = h.cw.opt.UnknownEncodingName
		}

		// Trim leading BOM bytes
		r.Content = util.TrimBOM(r.Content, name)

		r.Charset, r.CertainCharset, r.Encoding = name, certain, e
		if name != "utf-8" {
			if b, err := util.ConvToUTF8(r.Content, e); err == nil {
				r.Content = b
				r.CharsetDecoded = true
			}
		}
	}

	r.follow, r.links = h.cw.ctrl.Handle(r)
	r.CloseBody()
}

// OriginalContent converts response content to its original encoding.
func (resp *Response) OriginalContent() []byte {
	if !resp.CharsetDecoded {
		return resp.Content
	}
	b, _ := util.ConvTo(resp.Content, resp.Encoding)
	return b
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
