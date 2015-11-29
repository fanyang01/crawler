package crawler

import "sync"

type workerConn struct {
	nworker int
	wg      *sync.WaitGroup // managed by crawler
	quit    chan struct{}
}

func (c *workerConn) conn() *workerConn { return c }

type worker interface {
	conn() *workerConn
	work()
	cleanup()
}

func (cw *Crawler) initWorker(w worker, nworker int) {
	w.conn().nworker = nworker
	w.conn().wg = &cw.wg
	w.conn().quit = cw.quit
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
