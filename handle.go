package crawler

import (
	"net/url"
	"sync"
)

type Handler interface {
	Handle(resp *Response) (follow bool)
}

type handlerQuery struct {
	url   *url.URL
	reply chan Handler
}

type handler struct {
	In      <-chan *Response
	Out     chan *Response
	Req     chan<- *ctrlQuery
	Done    chan struct{}
	nworker int
}

func newHandler(nworker int, in <-chan *Response, done chan struct{},
	ch chan<- *ctrlQuery) *handler {
	return &handler{
		In:   in,
		Req:  ch,
		Out:  make(chan *Response, nworker),
		Done: done,
	}
}

func (h *handler) start() {
	var wg sync.WaitGroup
	wg.Add(h.nworker)
	for i := 0; i < h.nworker; i++ {
		go func() {
			h.work()
			wg.Done()
		}()
	}
	go func() {
		wg.Wait()
		close(h.Out)
	}()
}

func (h *handler) work() {
	for r := range h.In {
		q := &ctrlQuery{
			url:   r.Locations,
			reply: make(chan Controller),
		}
		h.Req <- q
		H := <-q.reply
		follow := H.Handle(r)
		r.CloseBody()
		if !follow {
			r = nil // downstream should check nil
		}
		select {
		case h.Out <- r:
		case <-h.Done:
			return
		}
	}
}
