package crawler

import (
	"log"
	"net/http"
	"net/url"
)

type Request struct {
	Client *http.Client
	*http.Request
}

type requestMaker struct {
	cw     *Crawler
	In     chan url.URL
	Out    chan *Request
	opt    *Option
	setter requestSetter
}

type requestSetter interface {
	SetRequest(*Request)
}

func newRequestMaker(cw *Crawler, opt *Option) *requestMaker {
	return &requestMaker{
		cw:  cw,
		Out: make(chan *Request, opt.RequestMaker.OutQueueLen),
		opt: opt,
	}
}

func (rm *requestMaker) newRequest(u url.URL) (req *Request, err error) {
	u.Fragment = ""
	req = &Request{
		Client: rm.opt.DefaultClient,
	}
	if req.Request, err = http.NewRequest("GET", u.String(), nil); err != nil {
		return
	}

	req.Header.Set("User-Agent", rm.opt.RobotoAgent)
	rm.setter.SetRequest(req)
	return
}

func (rm *requestMaker) Start(setter requestSetter) {
	rm.setter = setter
	go func() {
		for u := range rm.In {
			if req, err := rm.newRequest(u); err == nil {
				rm.Out <- req
			} else {
				log.Println(err)
			}
		}
		close(rm.Out)
	}()
}
