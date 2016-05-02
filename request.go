package crawler

import (
	"net/http"
	"net/url"
	"strings"
)

type maker struct {
	workerConn
	In     <-chan *url.URL
	ErrOut chan<- *url.URL
	Out    chan *Request
	cw     *Crawler
}

func (cw *Crawler) newRequestMaker() *maker {
	nworker := cw.opt.NWorker.Maker
	this := &maker{
		Out: make(chan *Request, nworker),
		cw:  cw,
	}
	cw.initWorker(this, nworker)
	return this
}

func (m *maker) newRequest(u *url.URL) (req *Request, err error) {
	req = &Request{
		ctx: newContext(m.cw, u),
	}
	if req.Request, err = http.NewRequest("GET", u.String(), nil); err != nil {
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
		errOut chan<- *url.URL
		req    *Request
		err    error
	)
	for u := range m.In {
		out, errOut = m.Out, nil
		if req, err = m.newRequest(u); err != nil {
			out, errOut = nil, m.ErrOut
			m.cw.log.Errorf("maker: %v", err)
		}
		select {
		case out <- req:
		case errOut <- u:
		case <-m.quit:
			return
		}
	}
}
