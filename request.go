package crawler

import (
	"net/http"
	"net/url"

	log "github.com/Sirupsen/logrus"
)

type maker struct {
	workerConn
	In  <-chan *url.URL
	Out chan *Request
	ctl Controller
}

func (cw *Crawler) newRequestMaker() *maker {
	nworker := cw.opt.NWorker.Maker
	this := &maker{
		Out: make(chan *Request, nworker),
		ctl: cw.ctl,
	}
	this.nworker = nworker
	this.wg = &cw.wg
	this.quit = cw.quit
	return this
}

func (rm *maker) newRequest(u *url.URL) (req *Request, err error) {
	req = &Request{}
	if req.Request, err = http.NewRequest("GET", u.String(), nil); err != nil {
		return
	}
	rm.ctl.Prepare(req)

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
			log.Errorln(err)
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
