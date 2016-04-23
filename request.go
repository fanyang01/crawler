package crawler

import (
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/net/context"
)

type maker struct {
	workerConn
	In  <-chan *url.URL
	Out chan *Request
	cw  *Crawler
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

func (rm *maker) newRequest(u *url.URL) (req *Request, err error) {
	req = &Request{
		Context: context.Background(),
	}
	if req.Request, err = http.NewRequest("GET", u.String(), nil); err != nil {
		return
	}
	rm.cw.ctrl.Prepare(req)

	req.Method = strings.ToUpper(req.Method)
	if req.Client == nil {
		req.Client = DefaultClient
	}
	return
}

func (rm *maker) cleanup() { close(rm.Out) }

func (rm *maker) work() {
	for u := range rm.In {
		req, err := rm.newRequest(u)
		if err != nil {
			log.Errorf("make request: %v", err)
			continue
		}
		select {
		case rm.Out <- req:
		case <-rm.quit:
			return
		}
	}
}
