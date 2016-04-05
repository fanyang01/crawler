package crawler

import (
	"fmt"
	"net/url"
	"sync"
	"time"
)

const (
	PQueueLen     int = 4096
	MaxRetryTimes     = 5
)

type scheduler struct {
	workerConn
	NewIn  chan *url.URL
	DoneIn chan *url.URL
	ErrIn  chan *url.URL
	Out    chan *url.URL
	ResIn  chan *Response
	cw     *Crawler

	prioQueue PQ
	pqIn      chan<- *SchedItem
	pqOut     <-chan *SchedItem

	retry time.Duration // duration between retry
	once  sync.Once     // used for closing Out
	done  chan struct{}
}

func (cw *Crawler) newScheduler() *scheduler {
	nworker := cw.opt.NWorker.Scheduler
	pq := NewMemQueue(PQueueLen)
	chIn, chOut := pq.Channel()
	this := &scheduler{
		NewIn:     make(chan *url.URL, nworker),
		DoneIn:    make(chan *url.URL, nworker),
		ErrIn:     make(chan *url.URL, nworker),
		Out:       make(chan *url.URL, 4*nworker),
		cw:        cw,
		prioQueue: pq,
		pqIn:      chIn,
		pqOut:     chOut,
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
	wg.Add(sd.nworker)
	for i := 0; i < sd.nworker; i++ {
		go func() {
			sd.work()
			wg.Done()
		}()
	}

	go func() {
		wg.Wait()
		close(sd.Out)
		sd.wg.Done()
	}()
}

func (sd *scheduler) work() {
	var u *url.URL
	var item *SchedItem
	for {
		select {
		// Input:
		case resp := <-sd.ResIn:
			sd.cw.store.IncNTime()
			for _, link := range resp.links {
				if link.follow {
					item, _, err := sd.schedURL(link.URL, true)
					if err != nil {
						return // TODO
					}
					sd.pqIn <- item
				}
			}
			item, done, err := sd.schedURL(resp.RequestURL, false)
			if err != nil {
				return // TODO
			} else if !done {
				sd.pqIn <- item
				continue // for loop
			}
			if sd.cw.store.AllFinished() {
				sd.stop()
				return
			}
		case u = <-sd.NewIn:
			item, _, err := sd.schedURL(u, true)
			if err != nil {
				return // TODO
			}
			sd.pqIn <- item
		case u = <-sd.DoneIn:
			sd.cw.store.SetStatus(u, URLfinished)
			if sd.cw.store.AllFinished() {
				sd.stop()
				return
			}
		case u = <-sd.ErrIn:
			if cnt := sd.cw.store.IncErrCount(u); cnt >= sd.cw.opt.MaxRetry {
				sd.cw.store.SetStatus(u, URLerror)
				continue
			}
			sd.pqIn <- &SchedItem{
				URL:  u,
				Next: time.Now().Add(sd.retry),
			}
		// Output:
		case item = <-sd.pqOut:
			sd.Out <- item.URL
		// Control:
		case <-sd.done:
			return
		case <-sd.quit:
			sd.once.Do(func() {
				sd.prioQueue.Close()
			})
			return
		}
	}
}

func (sd *scheduler) stop() {
	sd.once.Do(func() {
		sd.prioQueue.Close()
		close(sd.done)
	})
}

func (sd *scheduler) schedURL(u *url.URL, isNew bool) (item *SchedItem, done bool, err error) {
	uu, ok := sd.cw.store.Get(u)
	if !ok {
		err = fmt.Errorf("%s should be in store", u.String())
		return
	}
	minTime := uu.Visited.LastTime.Add(sd.cw.opt.MinDelay)
	item = &SchedItem{
		URL: u,
	}
	if item.Score, item.Next, done = sd.cw.ctrl.Schedule(uu); done {
		sd.cw.store.SetStatus(u, URLfinished)
		return
	}
	if item.Next.Before(minTime) {
		item.Next = minTime
	}
	return
}
