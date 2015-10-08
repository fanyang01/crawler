package crawler

import (
	"net/url"
	"time"
)

const (
	PQueueLen int = 4096
	TQueueLen     = 4096
	EQueueLen     = 512
)

type scheduler struct {
	In      chan *url.URL
	Out     chan *url.URL
	Done    chan struct{}
	nworker int
	pQueue  *pqueue
	tQueue  *tqueue
	eQueue  chan *url.URL
}

func newScheduler(nworker int, in chan *url.URL, done chan struct{},
	query chan schedQuery, store URLStore) *scheduler {

	return &scheduler{
		In:      in,
		Done:    done,
		Out:     make(chan *url.URL, nworker),
		nworker: nworker,
		pQueue:  newPQueue(PQueueLen),
		tQueue:  newTQueue(TQueueLen),
		eQueue:  make(chan *url.URL, EQueueLen),
	}
}

func (sched *scheduler) start() {
	// Move available URL to priority queue from time queue
	go func() {
		duration := 100 * time.Millisecond
		for {
			if !c.tQueue.IsAvailable() {
				time.Sleep(duration)
				if duration < 5*1E9 {
					duration = duration * 2
				}
				continue
			}
			if urls, ok := sched.tQueue.MultiPop(); ok {
				for _, u := range urls {
					sched.pQueue.Push(u)
				}
				duration = 100 * time.Millisecond
			}
		}
	}()
	// Pop URL from priority queue
	go func() {
		for {
			select {
			case sched.Out <- sched.pQueue.Pop():
			case <-sched.done:
				close(sched.Out)
				return
			}
		}
	}()
}
