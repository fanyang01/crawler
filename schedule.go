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
)

type scheduler struct {
	New     chan *url.URL
	Fetched chan *url.URL
	Out     chan *url.URL
	Done    chan struct{}
	query   chan *ctrlQuery
	nworker int
	store   URLStore
	pQueue  *pqueue
	tQueue  *tqueue
	eQueue  chan *url.URL
	pool    sync.Pool
}

func newScheduler(nworker int, newIn chan *url.URL, fetchedIn chan *url.URL,
	done chan struct{}, query chan *ctrlQuery, store URLStore,
	out chan *url.URL) *scheduler {

	return &scheduler{
		New:     newIn,
		Fetched: fetchedIn,
		Done:    done,
		Out:     out,
		query:   query,
		nworker: nworker,
		store:   store,
		pQueue:  newPQueue(PQueueLen),
		tQueue:  newTQueue(TQueueLen),
		eQueue:  make(chan *url.URL, EQueueLen),
		pool: sync.Pool{
			New: func() interface{} {
				return &URL{}
			},
		},
	}
}

func (sched *scheduler) start() {
	var wg sync.WaitGroup
	wg.Add(sched.nworker)
	for i := 0; i < sched.nworker; i++ {
		go func() {
			sched.work()
			wg.Done()
		}()
	}
	go func() {
		wg.Wait()
		close(sched.Out)
	}()

	go sched.popTQ()
	go sched.retry()
	// Pop URL from priority queue
	// TODO: this goroutine may block when crawler stops.
	go func() {
		for {
			u := sched.pQueue.Pop() // Pop will block when queue is empty
			select {
			case sched.Out <- u.Loc:
				sched.pool.Put(u)
			case <-sched.Done:
				return
			}
		}
	}()
}

func (sched *scheduler) work() {
	for {
		var u *url.URL
		select {
		case u = <-sched.New:
		case u = <-sched.Fetched:
		case <-sched.Done:
			return
		}
		query := &ctrlQuery{
			url:   u,
			reply: make(chan Controller),
		}
		sched.query <- query
		SC := <-query.reply
		h := sched.store.Watch(*u)
		if h == nil {
			continue
		}
		uu := h.V()
		uu.Score, uu.nextTime = SC.Schedule(*uu)
		uuu := sched.pool.Get().(*URL)
		*uuu = *uu
		h.Unlock()
		if uuu.nextTime.After(time.Now()) {
			sched.tQueue.Push(uuu)
		} else {
			sched.pQueue.Push(uuu)
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
		if !sched.tQueue.IsAvailable() {
			time.Sleep(duration)
			if duration < 5*time.Second {
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
}

// Periodically retry urls in error queue
func (sched *scheduler) retry() {
	for {
		select {
		case u := <-sched.eQueue:
			uu := newURL(*u)
			uu.nextTime = time.Now().Add(RetryDelay)
			sched.tQueue.Push(uu)
		case <-sched.Done:
			return
		}
	}
}
