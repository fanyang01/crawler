package crawler

import (
	"log"
	"net/http"
)

var (
	RespBufSize = 64
)

type fetcher struct {
	cache   *cachePool
	client  *http.Client
	option  *Option
	workers chan struct{}
	In      chan *Request
	Out     chan *Response
}

func newFetcher(opt *Option) (fc *fetcher) {
	fc = &fetcher{
		option:  opt,
		Out:     make(chan *Response, opt.Fetcher.OutQueueLen),
		cache:   newCachePool(),
		workers: make(chan struct{}, opt.Fetcher.NumOfWorkers),
	}
	return
}

func (fc *fetcher) do(req *Request) {
	// First check cache
	if resp, ok := fc.cache.Get(req.url); ok {
		fc.Out <- resp
		return
	}
	resp, err := fc.fetch(req)
	if err != nil {
		log.Printf("fetcher: %v\n", err)
		return
	}
	// Add to cache
	fc.cache.Add(resp)
	fc.Out <- resp
}

func (fc *fetcher) Start() {
	for i := 0; i < fc.option.Fetcher.NumOfWorkers; i++ {
		fc.workers <- struct{}{}
	}
	go func() {
		for req := range fc.In {
			<-fc.workers
			go func(r *Request) {
				fc.do(r)
				fc.workers <- struct{}{}
			}(req)
		}
		close(fc.Out)
	}()
}
