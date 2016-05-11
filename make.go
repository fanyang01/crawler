package crawler

import (
	"net/http"
	"strings"
)

type maker struct {
	workerConn
	In        <-chan *Context
	ErrOut    chan<- *Context
	CancelOut chan<- *Context
	Out       chan *Request
	cw        *Crawler
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
		return nil, err
	}
	m.cw.ctrl.Prepare(req)
	if err = req.ctx.err; err != nil {
		return nil, err
	}

	req.Method = strings.ToUpper(req.Method)
	if req.Client == nil {
		req.Client = DefaultClient
	}
	return
}

func (m *maker) cleanup() { close(m.Out) }

func (m *maker) work() {
	for ctx := range m.In {
		var (
			logger    = m.logger.New("url", ctx.url)
			out       = m.Out
			errOut    chan<- *Context
			cancelOut chan<- *Context
			req       *Request
			err       error
		)
		if req, err = m.newRequest(ctx); err != nil {
			out, errOut = nil, m.ErrOut
			logger.Error("make request", "err", err)
		} else if req.cancel {
			out, cancelOut = nil, m.CancelOut
			logger.Info("request canceled")
		}
		select {
		case out <- req:
		case errOut <- ctx:
		case cancelOut <- ctx:
		case <-m.quit:
			return
		}
	}
}
