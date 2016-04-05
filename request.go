package crawler

import (
	"net/http"
	"net/url"

	"github.com/Sirupsen/logrus"
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
	req = &Request{}
	if req.Request, err = http.NewRequest("GET", u.String(), nil); err != nil {
		return
	}
	rm.cw.ctrl.Prepare(req)

	if req.Client == nil {
		switch req.Type {
		case ReqDynamic:
			req.Client = DefaultAjaxClient
		case ReqStatic:
			fallthrough
		default:
			req.Client = DefaultClient
		}
	}
	return
}

func (rm *maker) cleanup() { close(rm.Out) }

func (rm *maker) work() {
	for u := range rm.In {
		if req, err := rm.newRequest(u); err != nil {
			logrus.Errorln(err)
			continue
		} else {
			select {
			case rm.Out <- req:
			case <-rm.quit:
				return
			}
		}

	}
}
