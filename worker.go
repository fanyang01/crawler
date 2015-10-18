package crawler

import "sync"

type conn struct {
	nworker int
	WG      *sync.WaitGroup // managed by crawler
	Done    chan struct{}
}

func (c *conn) Conn() *conn { return c }

type worker interface {
	Conn() *conn
	work()
	cleanup()
}

func start(w worker) {
	var wg sync.WaitGroup
	wg.Add(w.Conn().nworker)
	for i := 0; i < w.Conn().nworker; i++ {
		go func() {
			w.work()
			wg.Done()
		}()
	}
	go func() {
		wg.Wait()
		w.cleanup()
		w.Conn().WG.Done()
	}()
}
