package crawler

import (
	"log"
	"net/http"
	"net/url"
	"sync"
)

type makerQuery struct {
	url   *url.URL
	reply chan requestSetter
}

type Request struct {
	Client Client
	*http.Request
}

type requestMaker struct {
	query   chan<- makerQuery
	In      chan url.URL
	Out     chan *Request
	Done    chan struct{}
	nworker int
}

type requestSetter interface {
	SetRequest(*Request)
}

func newRequestMaker(nworker int, in <-chan url.URL, done chan struct{},
	query chan<- makerQuery) *requestMaker {
	return &requestMaker{
		query:   query,
		Out:     make(chan *Request, nworker),
		Done:    done,
		In:      in,
		nworker: nworker,
	}
}

func (rm *requestMaker) newRequest(u url.URL) (req *Request, err error) {
	u.Fragment = ""
	req = &Request{
		Client: DefaultClient,
	}
	if req.Request, err = http.NewRequest("GET", u.String(), nil); err != nil {
		return
	}
	req.Header.Set("User-Agent", rm.opt.RobotoAgent)
	query := makerQuery{
		reply: make(chan requestSetter),
		url:   &u,
	}
	rm.query <- query
	S := <-query.reply
	S.SetRequest(req)
	return
}

func (rm *requestMaker) start() {
	var wg sync.WaitGroup
	wg.Add(nworker)
	for i := 0; i < rm.nworker; i++ {
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
		}
		select {
		case rm.Out <- req:
		case <-rm.Done:
			return
		}

	}
}
