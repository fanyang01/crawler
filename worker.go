package crawler

import (
	"sync"

	"gopkg.in/inconshreveable/log15.v2"
)

type workerConn struct {
	nworker int
	logger  log15.Logger
	wg      *sync.WaitGroup // managed by crawler
	quit    chan struct{}
}

func (c *workerConn) conn() *workerConn { return c }

type worker interface {
	conn() *workerConn
	work()
	cleanup()
}

func (cw *Crawler) initWorker(name string, w worker, nworker int) {
	w.conn().nworker = nworker
	w.conn().wg = &cw.wg
	w.conn().quit = cw.quit
	w.conn().logger = cw.logger.New("worker", name)
}

func start(w worker) {
	var wg sync.WaitGroup
	wg.Add(w.conn().nworker)
	for i := 0; i < w.conn().nworker; i++ {
		go func() {
			w.work()
			wg.Done()
		}()
	}
	go func() {
		wg.Wait()
		w.cleanup()
		w.conn().wg.Done()
	}()
}
