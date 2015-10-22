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
	DoneIn    chan *url.URL
	ErrIn     chan *url.URL
	Out       chan *url.URL
	ResIn     chan *Link
	cw        *Crawler
	ctrler    Controller
	store     URLStore
	prioQueue PQ
	waitQueue WQ
	sites     sites
	stat      *Statistic
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
		store:     cw.urlStore,
		ctrler:    cw.ctrler,
		prioQueue: newPQueue(PQueueLen),
		waitQueue: newWQueue(TQueueLen),
		stat:      &cw.statistic,
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
		case link := <-sd.ResIn:
			sd.stat.IncTimesCount()
			for _, anchor := range link.Anchors {
				if anchor.ok {
					sd.enqueueNew(anchor.URL)
					anchor.URL = nil
				}
			}
			if done := sd.enqueueAgain(link.Base); done {
				if alldone := sd.stat.IncDoneCount(); alldone {
					sd.stop()
					return
				}
			}
			link.Base = nil
		case u = <-sd.DoneIn:
			if alldone := sd.stat.IncDoneCount(); alldone {
				sd.stop()
				return
			}
		case u = <-sd.NewIn:
			sd.enqueueNew(u)
		case u = <-sd.ErrIn:
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
	uu, ok := sd.store.Get(u)
	if !ok {
		panic("store fault")
	}
	minTime := uu.Visited.Time.Add(MinDelay)
	item.Score, item.Next, _ = sd.ctrler.Schedule(uu)
	if item.Next.Before(minTime) {
		item.Next = minTime
	}
	sd.stat.IncAllCount()
	if item.Next.After(time.Now()) {
		sd.waitQueue.Push(item)
	} else {
		sd.prioQueue.Push(item)
	}
}

func (sd *scheduler) enqueueAgain(u *url.URL) (done bool) {
	uu, ok := sd.store.Get(u)
	if !ok {
		panic("store fault")
	}
	minTime := uu.Visited.Time.Add(MinDelay)

	item := &SchedItem{
		URL: u,
	}
	if item.Score, item.Next, done = sd.ctrler.Schedule(uu); done {
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
