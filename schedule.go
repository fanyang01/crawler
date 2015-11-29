package crawler

import (
	"net/url"
	"sync"
	"time"
)

const (
	PQueueLen     int = 4096
	TQueueLen         = 4096
	MaxRetryTimes     = 5
)

type scheduler struct {
	workerConn
	NewIn     chan *url.URL
	DoneIn    chan *url.URL
	ErrIn     chan *url.URL
	Out       chan *url.URL
	ResIn     chan *Response
	cw        *Crawler
	prioQueue PQ
	waitQueue WQ
	retry     time.Duration // duration between retry
	once      sync.Once     // used for closing Out
	done      chan struct{}
}

type SchedItem struct {
	URL   *url.URL
	Next  time.Time
	Score int
}

func (cw *Crawler) newScheduler() *scheduler {
	nworker := cw.opt.NWorker.Scheduler
	this := &scheduler{
		NewIn:     make(chan *url.URL, nworker),
		DoneIn:    make(chan *url.URL, nworker),
		ErrIn:     make(chan *url.URL, nworker),
		Out:       make(chan *url.URL, 4*nworker),
		cw:        cw,
		prioQueue: newPQueue(PQueueLen),
		waitQueue: newWQueue(TQueueLen),
		retry:     cw.opt.RetryDuration,
		done:      make(chan struct{}),
	}

	this.nworker = nworker
	this.wg = &cw.wg
	this.quit = cw.quit

	return this
}

func (sd *scheduler) start() {
	var wg sync.WaitGroup
	wg.Add(sd.nworker + 2)
	for i := 0; i < sd.nworker; i++ {
		go func() {
			sd.work()
			wg.Done()
		}()
	}

	go func() {
		sd.popTQ()
		wg.Done()
	}()
	// Pop URL from priority queue
	go func() {
		sd.popPQ()
		wg.Done()
	}()

	go func() {
		wg.Wait()
		close(sd.Out)
		sd.wg.Done()
	}()
}

func (sd *scheduler) work() {
	for {
		var u *url.URL
		select {
		case resp := <-sd.ResIn:
			sd.cw.store.IncNTime()
			for _, anchor := range resp.links {
				if anchor.follow {
					sd.enqueueNew(anchor.URL)
					anchor.URL = nil
				}
			}
			if done := sd.enqueueAgain(resp.RequestURL); done {
				if sd.cw.store.AllFinished() {
					sd.stop()
					return
				}
			}
		case u = <-sd.DoneIn:
			sd.cw.store.SetStatus(u, URLfinished)
			if sd.cw.store.AllFinished() {
				sd.stop()
				return
			}
		case u = <-sd.NewIn:
			sd.enqueueNew(u)
		case u = <-sd.ErrIn:
			if cnt := sd.cw.store.IncErrCount(u); cnt >= sd.cw.opt.MaxRetry {
				sd.cw.store.SetStatus(u, URLerror)
				continue
			}
			sd.waitQueue.Push(&SchedItem{
				URL:  u,
				Next: time.Now().Add(sd.retry),
			})
		case <-sd.done:
			return
		case <-sd.quit:
			sd.once.Do(func() {
				sd.prioQueue.Close()
				sd.waitQueue.Close()
			})
			return
		}
	}
}

func (sd *scheduler) stop() {
	sd.once.Do(func() {
		sd.prioQueue.Close()
		sd.waitQueue.Close()
		close(sd.done)
	})
}

func (sd *scheduler) enqueueNew(u *url.URL) {
	item := &SchedItem{
		URL: u,
	}
	uu, ok := sd.cw.store.Get(u)
	if !ok {
		panic("store fault")
	}
	minTime := uu.Visited.LastTime.Add(sd.cw.opt.MinDelay)
	item.Score, item.Next, _ = sd.cw.ctl.Schedule(uu)
	if item.Next.Before(minTime) {
		item.Next = minTime
	}
	if item.Next.After(time.Now()) {
		sd.waitQueue.Push(item)
	} else {
		sd.prioQueue.Push(item)
	}
}

func (sd *scheduler) enqueueAgain(u *url.URL) (done bool) {
	uu, ok := sd.cw.store.Get(u)
	if !ok {
		panic("store fault")
	}
	minTime := uu.Visited.LastTime.Add(sd.cw.opt.MinDelay)

	item := &SchedItem{
		URL: u,
	}
	if item.Score, item.Next, done = sd.cw.ctl.Schedule(uu); done {
		sd.cw.store.SetStatus(u, URLfinished)
		return
	}
	if item.Next.Before(minTime) {
		item.Next = minTime
	}

	if item.Next.After(time.Now()) {
		sd.waitQueue.Push(item)
	} else {
		sd.prioQueue.Push(item)
	}
	return false
}

func (sd *scheduler) popPQ() {
	for {
		item := sd.prioQueue.Pop()
		if item == nil { // Pop will return nil if the queue is closed.
			return
		}
		select {
		case sd.Out <- item.URL:
			item.URL = nil
		case <-sd.done:
			return
		case <-sd.quit:
			return
		}
	}
}

// Move available URL to priority queue from time queue
func (sd *scheduler) popTQ() {
	duration := 100 * time.Millisecond
	for {
		select {
		case <-sd.quit:
			return
		case <-sd.done:
			return
		default:
			if !sd.waitQueue.IsAvailable() {
				time.Sleep(duration)
				if duration < 2*time.Second {
					duration = duration * 2
				}
				continue
			}
			if items, ok := sd.waitQueue.MultiPop(); ok {
				for _, i := range items {
					sd.prioQueue.Push(i)
				}
				duration = 100 * time.Millisecond
			}
		}
	}
}
