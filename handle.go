package crawler

import (
	"errors"
	"fmt"

	"github.com/PuerkitoBio/goquery"
)

var ErrNotHTML = errors.New("content type is not HTML")

type handler struct {
	workerConn
	In  <-chan *Response
	Out chan *Response
	cw  *Crawler
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
		if r.err == nil {
			if err := h.handle(r); err != nil {
				r.err = fmt.Errorf("handler: %v")
			}
			r.bodyCloser.Close()
		}
		select {
		case h.Out <- r:
		case <-h.quit:
			return
		}
	}
}

func (h *handler) handle(r *Response) error {
	depth, err := r.ctx.Depth()
	if err != nil {
		return err
	}
	ch := make(chan *Link, LinkPerPage)
	go func() {
		h.cw.ctrl.Handle(r, ch)
		close(ch)
	}()
	return h.handleLink(r, ch, depth)
}

func (h *handler) handleLink(r *Response, ch <-chan *Link, depth int) error {
	// Treat the new url as one found under the original url
	original := r.URL.String()
	if r.NewURL.String() != original {
		if err := h.filter(r, &Link{
			URL:   r.NewURL,
			Depth: depth + 1,
		}); err != nil {
			return err
		}
	}
	if refresh := r.Refresh.URL; refresh != nil && refresh.String() != original {
		if err := h.filter(r, &Link{
			URL:   r.Refresh.URL,
			Depth: depth + 1,
		}); err != nil {
			return err
		}
	}
	for link := range ch {
		link.Depth = depth
		if err := h.filter(r, link); err != nil {
			return err
		}
	}
	return nil
}

func (h *handler) filter(r *Response, link *Link) error {
	if !h.cw.ctrl.Accept(r.ctx, link) {
		return nil
	}
	if ok, err := h.cw.store.Exist(link.URL); err != nil {
		return err
	} else if ok {
		return nil
	}
	// New link
	if ok, err := h.cw.store.PutNX(&URL{
		Loc:   *link.URL,
		Depth: link.Depth,
	}); err != nil {
		return err
	} else if ok {
		r.links = append(r.links, link)
	}
	return nil
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
