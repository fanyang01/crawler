package crawler

import "sync"

type workerConn struct {
	nworker int
	WG      *sync.WaitGroup // managed by crawler
	Done    chan struct{}
}

func (c *workerConn) conn() *workerConn { return c }

type worker interface {
	conn() *workerConn
	work()
	cleanup()
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
		w.conn().WG.Done()
	}()
}
