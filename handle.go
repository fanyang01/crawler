package crawler

import (
	"errors"
	"io"
	"io/ioutil"
	"net/url"

	"golang.org/x/net/html/charset"

	"github.com/PuerkitoBio/goquery"
	"github.com/Sirupsen/logrus"
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
		r.bodyCloser.Close()
		select {
		case h.Out <- r:
		case <-h.quit:
			return
		}
	}
}

func (h *handler) handle(r *Response) {
	if !CT_HTML.match(r.ContentType) {
		r.links = h.cw.ctrl.Handle(r)
		return
	}

	e, name, certain := charset.DetermineEncoding(r.pview, r.ContentType)
	// according to charset package source, default unknown charset is windows-1252.
	if !certain && name == "windows-1252" {
		label := h.cw.ctrl.Charset(r.URL)
		if e, name = charset.Lookup(label); e == nil {
			logrus.Warn("unsupported charset:", label)
		} else {
			certain = true
		}
	}
	r.Charset, r.CertainCharset, r.Encoding = name, certain, e
	if name != "" && e != nil {
		r.Body, _ = util.NewUTF8Reader(name, r.Body)
	}

	depth := h.cw.store.GetDepth(r.URL)

	if follow := h.cw.ctrl.Follow(r, depth); !follow {
		r.links = h.cw.ctrl.Handle(r)
		return
	}

	rs, chCopy := util.DumpReader(
		io.LimitReader(r.Body, h.cw.opt.MaxHTML), 2,
	)
	r.Body = rs[0]
	chFind := make(chan struct{}, 1)
	go h.find(r, rs[1], chFind)

	links := h.cw.ctrl.Handle(r)

	io.Copy(ioutil.Discard, r.Body)
	<-chCopy
	<-chFind
	r.links = append(r.links, links...)
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
	if doc, err = goquery.NewDocumentFromReader(resp.Body); err != nil {
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
