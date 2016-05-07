package crawler

import (
	"net/http"
	"net/url"
	"strings"
)

// Request is a HTTP request to be made.
type Request struct {
	*http.Request
	Proxy   *url.URL
	Cookies []*http.Cookie
	Client  Client
	ctx     *Context
}

func (r *Request) Context() *Context { return r.ctx }

type maker struct {
	workerConn
	In     <-chan *Context
	ErrOut chan<- *Context
	Out    chan *Request
	cw     *Crawler
}

func (cw *Crawler) newRequestMaker() *maker {
	nworker := cw.opt.NWorker.Maker
	this := &maker{
		Out: make(chan *Request, nworker),
		cw:  cw,
	}
	cw.initWorker("maker", this, nworker)
	return this
}

func (m *maker) newRequest(ctx *Context) (req *Request, err error) {
	req = &Request{ctx: ctx}
	if req.Request, err = http.NewRequest("GET", ctx.url.String(), nil); err != nil {
		return
	}
	m.cw.ctrl.Prepare(req)

	req.Method = strings.ToUpper(req.Method)
	if req.Client == nil {
		req.Client = DefaultClient
	}
	return
}

func (m *maker) cleanup() { close(m.Out) }

func (m *maker) work() {
	var (
		out    chan<- *Request
		errOut chan<- *Context
		req    *Request
		err    error
	)
	for ctx := range m.In {
		out, errOut = m.Out, nil
		if req, err = m.newRequest(ctx); err != nil {
			out, errOut = nil, m.ErrOut
			m.logger.Error("make request", "err", err)
		}
		select {
		case out <- req:
		case errOut <- ctx:
		case <-m.quit:
			return
		}
	}
}
