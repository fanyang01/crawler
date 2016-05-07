package crawler

import (
	"bytes"
	"io"
	"net/url"
	"sync"

	"github.com/fanyang01/crawler/urlx"

	"golang.org/x/net/html"
)

type handler struct {
	workerConn
	cw *Crawler

	In       <-chan *Response
	Out      chan *Response
	ErrOut   chan *Response
	RetryOut chan *url.URL

	stop chan struct{}
	once sync.Once
}

func (cw *Crawler) newRespHandler() *handler {
	nworker := cw.opt.NWorker.Handler
	this := &handler{
		cw:   cw,
		Out:  make(chan *Response, nworker),
		stop: make(chan struct{}),
	}
	cw.initWorker("handler", this, nworker)
	return this
}

func (h *handler) cleanup() { close(h.Out) }

func (h *handler) work() {
	for r := range h.In {
		var (
			err    error
			errOut chan *Response
			out    = h.Out
			logger = h.logger.New("url", r.URL)
		)
		if err = h.handle(r); err != nil {
			err, _ = r.ctx.Value(ckError).(error)
		}
		r.bodyCloser.Close()
		if err != nil {
			switch err := err.(type) {
			case StorageError:
				logger.Crit("storage fault", "err", err)
				h.exit()
				return
			case RetriableError:
				logger.Error("error occured, will retry...", "err", err)
			default:
				logger.Error("unknown error", "err", err)
			}
			r.err = err
			out, errOut = nil, h.ErrOut
		}
		select {
		case out <- r:
		case errOut <- r:
		case <-h.stop:
			return
		case <-h.quit:
			return
		}
	}
}

func (h *handler) exit() {
	h.once.Do(func() { close(h.stop) })
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
		if refresh := r.Refresh.URL; refresh != nil &&
			refresh.String() != original {
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
		if ok, err := h.filter(r, link, depth); err != nil {
			return err
		} else if ok {
			r.links = append(r.links, link)
		}
	}
	return nil
}

func (h *handler) filter(r *Response, link *Link, depth int) (bool, error) {
	if !h.cw.ctrl.Accept(r, link) {
		return false, nil
	}
	if ok, err := h.cw.store.Exist(link.URL); err != nil {
		return false, err
	} else if ok {
		return false, nil
	}
	// New link
	if ok, err := h.cw.store.PutNX(&URL{
		URL:   *link.URL,
		Extra: link.Extra,
		Depth: depth + 1,
	}); err != nil || !ok {
		return false, err
	}
	return true, nil
}

func ExtractHref(base *url.URL, reader io.Reader, ch chan<- *Link) error {
	z := html.NewTokenizer(reader)
	f := func(z *html.Tokenizer, base *url.URL) *url.URL {
		for {
			key, val, more := z.TagAttr()
			if bytes.Equal(key, []byte("href")) {
				if u, err := urlx.ParseRef(base, string(val)); err == nil {
					return u
				}
				break
			}
			if !more {
				break
			}
		}
		return nil
	}
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
				if u := f(z, base); u != nil {
					ch <- &Link{URL: u}
				}
			}
		case html.SelfClosingTagToken:
			tn, hasAttr := z.TagName()
			if hasAttr && len(tn) == 4 && bytes.Equal(tn, []byte("base")) {
				if u := f(z, base); u != nil {
					base = u
				}
			}
		}
	}
	return nil
}
