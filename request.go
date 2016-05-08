package crawler

import (
	"net/http"
	"strings"
)

// Request is a HTTP request to be made.
type Request struct {
	*http.Request
	Client Client
	ctx    *Context
}

func (r *Request) Context() *Context {
	return r.ctx
}
func (r *Request) Use(c Client) {
	r.Client = c
}
func (r *Request) AddCookie(c *http.Cookie) {
	r.Request.AddCookie(c)
}
func (r *Request) AddHeader(key, value string) {
	r.Header.Add(key, value)
}
func (r *Request) SetHeader(key, value string) {
	r.Header.Set(key, value)
}
func (r *Request) SetBasicAuth(usr, pwd string) {
	r.Request.SetBasicAuth(usr, pwd)
}
func (r *Request) SetReferer(url string) {
	r.Header.Set("Referer", url)
}
func (r *Request) SetUserAgent(agent string) {
	r.Header.Set("User-Agent", agent)
}

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
