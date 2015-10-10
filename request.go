package crawler

import (
	"log"
	"net/http"
	"net/url"
	"sync"
)

const (
	RobotAgent = "I'm a robot"
)

type makerQuery struct {
}

type Request struct {
	Client Client
	*http.Request
}

type maker struct {
	query   chan<- *ctrlQuery
	In      <-chan *url.URL
	Out     chan *Request
	Done    chan struct{}
	nworker int
}

type requestSetter interface {
	SetRequest(*Request)
}

func newRequestMaker(nworker int, in <-chan *url.URL, done chan struct{},
	query chan<- *ctrlQuery) *maker {
	return &maker{
		query:   query,
		Out:     make(chan *Request, nworker),
		Done:    done,
		In:      in,
		nworker: nworker,
	}
}

func (rm *maker) newRequest(url *url.URL) (req *Request, err error) {
	u := *url
	u.Fragment = ""
	req = &Request{
		Client: StaticClient,
	}
	if req.Request, err = http.NewRequest("GET", u.String(), nil); err != nil {
		return
	}
	req.Header.Set("User-Agent", RobotAgent)
	query := &ctrlQuery{
		url:   &u,
		reply: make(chan Controller),
	}
	rm.query <- query
	S := <-query.reply
	S.SetRequest(req)
	return
}

func (rm *maker) start() {
	var wg sync.WaitGroup
	wg.Add(rm.nworker)
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
