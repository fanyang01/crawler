package crawler

import "net/http"

type Request struct {
	method, url string
	body        []byte
	client      *http.Client
	config      func(*http.Request)
}

type requestConstructor struct {
	In     chan *URL
	Out    chan *Request
	option *Option
}

func newRequestConstructor(opt *Option) *requestConstructor {
	return &requestConstructor{
		Out:    make(chan *Request, opt.RequestConstructor.OutQueueLen),
		option: opt,
	}
}

func newRequest(u *URL) *Request {
	return &Request{
		method: "GET",
		url:    u.Loc.String(),
	}
}

func (rc *requestConstructor) Start() {
	go func() {
		for u := range rc.In {
			rc.Out <- newRequest(u)
		}
		close(rc.Out)
	}()
}
