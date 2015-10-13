package crawler

import "sync"

type respHandler struct {
	In      <-chan *Response
	Out     chan *Response
	Req     chan<- *HandlerQuery
	Done    chan struct{}
	nworker int
}

func newRespHandler(nworker int, in <-chan *Response, done chan struct{},
	ch chan<- *HandlerQuery) *respHandler {
	return &respHandler{
		In:   in,
		Req:  ch,
		Out:  make(chan *Response, nworker),
		Done: done,
	}
}

func (h *respHandler) start() {
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

func (h *respHandler) work() {
	for r := range h.In {
		q := &HandlerQuery{
			URL:   r.Locations,
			Reply: make(chan Handler),
		}
		h.Req <- q
		H := <-q.Reply
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
