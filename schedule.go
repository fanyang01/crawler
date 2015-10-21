package crawler

import (
	"net/url"
	"sync"
	"time"
)

const (
	PQueueLen     int = 4096
	TQueueLen         = 4096
	RetryDelay        = time.Second * 30
	MaxRetryTimes     = 5
	MinDelay          = time.Second * 10
)

type scheduler struct {
	workerConn
	NewIn     chan *url.URL
	AgainIn   <-chan *url.URL
	ErrIn     <-chan *url.URL
	Out       chan *url.URL
	ctrler    Controller
	store     URLStore
	prioQueue PQ
	waitQueue WQ
	sites     sites
	retry     time.Duration // duration between retry
	once      sync.Once     // used for closing Out
}

type SchedItem struct {
	URL   *url.URL
	Next  time.Time
	Score int
}

func (cw *Crawler) newScheduler() *scheduler {
	nworker := cw.opt.NWorker.Scheduler
	this := &scheduler{
		Out:       make(chan *url.URL, nworker),
		store:     cw.urlStore,
		ctrler:    cw.ctrler,
		prioQueue: newPQueue(PQueueLen),
		waitQueue: newWQueue(TQueueLen),
		retry:     cw.opt.RetryDuration,
	}
	this.nworker = nworker
	this.WG = &cw.wg
	this.Done = cw.done
	return this
}

func (sched *scheduler) start() {
	var wg sync.WaitGroup
	wg.Add(sched.nworker + 2)
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
	// Pop URL from priority queue
	go func() {
		defer wg.Done()
		for {
			item := sched.prioQueue.Pop() // Pop will return nil if the queue is closed.
			if item == nil {
				return
			}
			select {
			case sched.Out <- item.URL:
				item.URL = nil
			case <-sched.Done:
				return
			}
		}
	}()

	go func() {
		wg.Wait()
		close(sched.Out)
		sched.WG.Done()
	}()
}

func (sched *scheduler) work() {
	for {
		var u *url.URL
		select {
		case u = <-sched.NewIn:
			sched.enqueue(u)
		case u = <-sched.AgainIn:
			sched.enqueue(u)
		case u = <-sched.ErrIn:
			sched.waitQueue.Push(&SchedItem{
				URL:  u,
				Next: time.Now().Add(sched.retry),
			})
		case <-sched.Done:
			sched.once.Do(func() {
				sched.prioQueue.Close()
				sched.waitQueue.Close()
			})
			return
		}
	}
}

func (sched *scheduler) enqueue(u *url.URL) {
	item := &SchedItem{
		URL: u,
	}
	var done bool
	uu, ok := sched.store.Get(u)
	if !ok {
		panic("store fault")
	}
	minTime := uu.Visited.Time.Add(MinDelay)
	item.Score, item.Next, done = sched.ctrler.Schedule(uu)
	if done {
		return
	}
	if item.Next.Before(minTime) {
		item.Next = minTime
	}
	if item.Next.After(time.Now()) {
		sched.waitQueue.Push(item)
	} else {
		sched.prioQueue.Push(item)
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
			if duration < 2*time.Second {
				duration = duration * 2
			}
			continue
		}
		if items, ok := sched.waitQueue.MultiPop(); ok {
			for _, i := range items {
				sched.prioQueue.Push(i)
			}
			duration = 100 * time.Millisecond
		}
	}
}
