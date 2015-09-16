package crawler

import (
	"log"
	"net/http"
)

var (
	RespBufSize = 64
)

type Pool struct {
	size    int
	workers []Worker
	free    chan *Worker
	client  *http.Client
	option  *Option
	In      chan *Request
	Out     chan *Response
}

type Worker struct {
	req  chan *Request
	resp chan *Response
	err  chan error
	pool *Pool
}

func newPool(opt *Option) (pool *Pool) {
	size := opt.Pool.Size
	pool = &Pool{
		size:    size,
		workers: make([]Worker, size),
		free:    make(chan *Worker, size),
		option:  opt,
		Out:     make(chan *Response, opt.Pool.OutQueueLen),
	}
	for i := 0; i < size; i++ {
		pool.workers[i] = Worker{
			req:  make(chan *Request),
			resp: make(chan *Response),
			err:  make(chan error),
			pool: pool,
		}
	}
	return
}

func (pool *Pool) Serve() {
	for i := 0; i < pool.size; i++ {
		go pool.workers[i].serve()
		pool.free <- &pool.workers[i]
	}
}

func (w *Worker) serve() {
	for req := range w.req {
		resp, err := w.fetch(req)
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

func (pool *Pool) acquire() *Worker {
	return <-pool.free
}

func (w *Worker) release() {
	w.pool.free <- w
}

// client
func (w *Worker) do(req *Request) (resp *Response, err error) {
	w.req <- req
	select {
	case resp = <-w.resp:
	case err = <-w.err:
	}
	return
}

func (pool *Pool) Start() {
	go func() {
		for req := range pool.In {
			go func(req *Request) {
				worker := pool.acquire()
				defer worker.release()
				resp, err := worker.do(req)
				if err != nil {
					log.Printf("do request: %v\n", err)
					return
				}
				pool.Out <- resp
			}(req)
		}
		close(pool.Out)
	}()
}
