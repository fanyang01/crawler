package crawler

import (
	"log"
	"net/http"
	"net/url"
)

const (
	RobotAgent = "I'm a robot"
)

// Request contains a client for doing this request.
type Request struct {
	Client Client
	*http.Request
}

type maker struct {
	conn
	In      <-chan *url.URL
	Out     chan *Request
	handler Handler
}

type requestSetter interface {
	SetRequest(*Request)
}

func newRequestMaker(nworker int, handler Handler) *maker {
	return &maker{
		Out:     make(chan *Request, nworker),
		handler: handler,
	}
}

func (rm *maker) newRequest(url *url.URL) (req *Request, err error) {
	u := *url
	u.Fragment = ""
	req = &Request{
		Client: DefaultClient,
	}
	if req.Request, err = http.NewRequest("GET", u.String(), nil); err != nil {
		return
	}
	req.Header.Set("User-Agent", RobotAgent)
	rm.handler.SetRequest(req)
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
			case <-rm.Done:
				return
			}
		}

	}
}
