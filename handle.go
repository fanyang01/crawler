package crawler

import (
	"bytes"
	"io"
	"net/url"

	"golang.org/x/net/html"
)

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
	cw.initWorker("handler", this, nworker)
	return this
}

func (h *handler) cleanup() { close(h.Out) }

func (h *handler) work() {
	for r := range h.In {
		if r.err == nil {
			var err error
			if err = h.handle(r); err == nil {
				err, _ = r.ctx.Value(ckError).(error)
			}
			if err != nil {
				h.logger.Error("handle response", "err", err)
				r.err = err
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
		original := r.URL.String()
		// Treat the new url as one found under the original url
		if r.NewURL.String() != original {
			newurl := *r.NewURL
			ch <- &Link{URL: &newurl}
		}
		if refresh := r.Refresh.URL; refresh != nil && refresh.String() != original {
			newurl := *refresh
			ch <- &Link{URL: &newurl}
		}
		h.cw.ctrl.Handle(r, ch)
		close(ch)
	}()
	return h.handleLink(r, ch, depth)
}

func (h *handler) handleLink(r *Response, ch <-chan *Link, depth int) error {
	for link := range ch {
		if err := h.cw.normalize(link.URL); err != nil {
			h.logger.Warn("normalize URL", "url", link.URL, "err", err)
			continue
		}
		link.depth = depth + 1
		link.hyperlink = (link.URL.Host != r.URL.Host)
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
		Depth: link.depth,
	}); err != nil {
		return err
	} else if ok {
		r.links = append(r.links, link)
	}
	return nil
}

func ExtractHref(base *url.URL, reader io.Reader, ch chan<- *Link) error {
	z := html.NewTokenizer(reader)
LOOP:
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			if err := z.Err(); err != io.EOF {
				return err
			}
			break LOOP
		case html.StartTagToken:
			tn, hasAttr := z.TagName()
			if hasAttr && len(tn) == 1 && tn[0] == 'a' {
				for {
					key, val, more := z.TagAttr()
					if bytes.Equal(key, []byte("href")) {
						if u, err := base.Parse(string(val)); err == nil {
							u.Fragment = ""
							ch <- &Link{
								URL: u,
							}
						}
						break
					}
					if !more {
						break
					}
				}
			}
		case html.SelfClosingTagToken:
			tn, hasAttr := z.TagName()
			if hasAttr && len(tn) == 4 && bytes.Equal(tn, []byte("base")) {
				for {
					key, val, more := z.TagAttr()
					if bytes.Equal(key, []byte("href")) {
						if u, err := base.Parse(string(val)); err == nil {
							base = u
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
	return nil
}
