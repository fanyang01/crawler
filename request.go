package crawler

import (
	"log"
	"net/http"
	"net/url"
	"sync"
)

type Request struct {
	Client Client
	*http.Request
}

type Client interface {
	Do(*Request) (*Response, error)
}

type requestMaker struct {
	client Client
	In     chan url.URL
	Out    chan *Request
	Done   chan struct{}
	opt    *Option
	setter requestSetter
}

type requestSetter interface {
	SetRequest(*Request)
}

func newRequestMaker(opt *Option, setter requestSetter) *requestMaker {
	return &requestMaker{
		Out:    make(chan *Request, opt.RequestMaker.QLen),
		client: NewStdClient(opt),
		opt:    opt,
		setter: setter,
	}
}

func (rm *requestMaker) newRequest(u url.URL) (req *Request, err error) {
	u.Fragment = ""
	req = &Request{
		Client: rm.client,
	}
	if req.Request, err = http.NewRequest("GET", u.String(), nil); err != nil {
		return
	}

	req.Header.Set("User-Agent", rm.opt.RobotoAgent)
	rm.setter.SetRequest(req)
	return
}

func (rm *requestMaker) Start() {
	n := rm.opt.RequestMaker.NWorker
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			rm.work()
			wg.Done()
		}()
	}
	go func() {
		wg.Wait()
		close(rm.Out)
	}()
}

func (rm *requestMaker) work() {
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
