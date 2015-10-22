package crawler

import (
	"log"
	"net/http"
	"net/url"
)

// Request contains a client for doing this request.
type Request struct {
	Client Client
	*http.Request
}

type maker struct {
	workerConn
	In     <-chan *url.URL
	Out    chan *Request
	ctrler Controller
}

func (cw *Crawler) newRequestMaker() *maker {
	nworker := cw.opt.NWorker.Maker
	this := &maker{
		Out:    make(chan *Request, nworker),
		ctrler: cw.ctrler,
	}
	this.nworker = nworker
	this.wg = &cw.wg
	this.quit = cw.quit
	return this
}

func (rm *maker) newRequest(u *url.URL) (req *Request, err error) {
	req = &Request{
		Client: DefaultClient,
	}
	if req.Request, err = http.NewRequest("GET", u.String(), nil); err != nil {
		return
	}
	rm.ctrler.Prepare(req)
	return
}

func (rm *maker) cleanup() { close(rm.Out) }

func (rm *maker) work() {
	for u := range rm.In {
		if req, err := rm.newRequest(u); err != nil {
			log.Println(err)
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
