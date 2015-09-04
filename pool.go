package crawler

import (
	"log"
	"net/http"
)

var (
	DefaultClient = http.DefaultClient
	RespBufSize   = 64
)

func NewPool(size int) (pool *Pool) {
	pool = &Pool{
		size:    size,
		workers: make([]Worker, size),
		free:    make(chan *Worker, size),
	}
	for i := 0; i < size; i++ {
		pool.workers[i] = Worker{
			req:  make(chan *Request),
			resp: make(chan *Response),
			err:  make(chan error),
			pool: pool,
		}
		go pool.workers[i].work()
		pool.free <- &pool.workers[i]
	}
	return
}

func (w *Worker) work() {
	for req := range w.req {
		resp, err := req.fetch()
		if err != nil {
			w.err <- err
			continue
		}
		w.resp <- resp
	}
}

func (pool *Pool) Destroy() {
	for i := 0; i < pool.size; i++ {
		close(pool.workers[i].req)
		close(pool.workers[i].resp)
	}
}

func (pool *Pool) Acquire() *Worker {
	return <-pool.free
}

func (w *Worker) Release() {
	w.pool.free <- w
}

// client
func (w *Worker) Do(req *Request) (resp *Response, err error) {
	w.req <- req
	select {
	case resp = <-w.resp:
	case err = <-w.err:
	}
	return
}

func (pool *Pool) DoRequest(req <-chan *Request) <-chan *Response {
	ch := make(chan *Response, RespBufSize)
	go pool.doRequest(req, ch)
	return ch
}

func (pool *Pool) doRequest(reqChan <-chan *Request, respChan chan<- *Response) {
	for req := range reqChan {
		go func(req *Request) {
			worker := pool.Acquire()
			defer worker.Release()
			resp, err := worker.Do(req)
			if err != nil {
				log.Printf("doRequest: %v\n", err)
				return
			}
			if resp.parse() {
				respChan <- resp
			}
		}(req)
	}
}
