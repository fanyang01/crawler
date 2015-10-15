package crawler

import (
	"net/url"
	"sync"
	"time"
)

const (
	PQueueLen  int = 4096
	TQueueLen      = 4096
	EQueueLen      = 512
	RetryDelay     = time.Second * 30
	MinDelay       = time.Second * 10
)

type scheduler struct {
	NewIn     chan *url.URL
	AgainIn   <-chan *url.URL
	Out       chan *url.URL
	Done      chan struct{}
	nworker   int
	handler   Handler
	store     URLStore
	prioQueue PQ
	waitQueue WQ
	ErrIn     chan *url.URL
	sites     sites
	pool      sync.Pool
}

func newScheduler(nworker int, handler Handler, store URLStore) *scheduler {

	return &scheduler{
		Out:       make(chan *url.URL, nworker),
		ErrIn:     make(chan *url.URL, EQueueLen),
		nworker:   nworker,
		store:     store,
		prioQueue: newPQueue(PQueueLen),
		waitQueue: newWQueue(TQueueLen),
		handler:   handler,
		pool: sync.Pool{
			New: func() interface{} {
				return &URL{}
			},
		},
	}
}

func (sched *scheduler) start() {
	var wg sync.WaitGroup
	wg.Add(sched.nworker + 3)
	for i := 0; i < sched.nworker; i++ {
		go func() {
			sched.work()
			wg.Done()
		}()
	}
	go func() {
		sched.popTQ()
		wg.Done()
	}()
	go func() {
		sched.retry()
		wg.Done()
	}()
	// Pop URL from priority queue
	go func() {
		defer wg.Done()
		for {
			u := sched.prioQueue.Pop() // Pop will block when queue is empty
			loc := u.Loc
			select {
			case sched.Out <- &loc:
				sched.pool.Put(u)
			case <-sched.Done:
				return
			}
		}
	}()

	go func() {
		wg.Wait()
		close(sched.Out)
	}()

}

func (sched *scheduler) work() {
	for {
		var u *url.URL
		select {
		case u = <-sched.NewIn:
		case u = <-sched.AgainIn:
		case <-sched.Done:
			// force sched.prioQueue.Pop() return, otherwise it may block forever
			sched.prioQueue.Push(&URL{})
			return
		}
		h := sched.store.Watch(*u)
		if h == nil {
			continue
		}
		uu := h.V()
		minTime := uu.Visited.Time.Add(MinDelay)
		uu.Score, uu.nextTime, uu.Done = sched.handler.Schedule(*uu)
		if !uu.Done && uu.Visited.Count > 0 && uu.nextTime.Before(minTime) {
			uu.nextTime = minTime
		}
		uuu := sched.pool.Get().(*URL)
		*uuu = *uu
		h.Unlock()
		if uuu.Done {
			continue
		}
		if uuu.nextTime.After(time.Now()) {
			sched.waitQueue.Push(uuu)
		} else {
			sched.prioQueue.Push(uuu)
		}
	}
}

// Move available URL to priority queue from time queue
func (sched *scheduler) popTQ() {
	duration := 100 * time.Millisecond
	for {
		select {
		case <-sched.Done:
			return
		default:
		}
		if !sched.waitQueue.IsAvailable() {
			time.Sleep(duration)
			if duration < 5*time.Second {
				duration = duration * 2
			}
			continue
		}
		if urls, ok := sched.waitQueue.MultiPop(); ok {
			for _, u := range urls {
				sched.prioQueue.Push(u)
			}
			duration = 100 * time.Millisecond
		}
	}
}

// Periodically retry urls in error queue
func (sched *scheduler) retry() {
	for {
		select {
		case u := <-sched.ErrIn:
			uu := newURL(*u)
			uu.nextTime = time.Now().Add(RetryDelay)
			sched.waitQueue.Push(uu)
		case <-sched.Done:
			return
		}
	}
}
