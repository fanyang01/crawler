package crawler

import (
	"net/url"
	"sync"
	"time"

	"github.com/fanyang01/crawler/queue"
)

const (
	PQueueLen     int = 4096
	MaxRetryTimes     = 5
)

type scheduler struct {
	workerConn
	NewIn     chan *url.URL
	DoneIn    chan *url.URL
	ErrIn     chan *url.URL
	RecoverIn chan *url.URL
	Out       chan *url.URL
	ResIn     chan *Response
	cw        *Crawler

	prioQueue queue.WaitQueue
	pqIn      chan<- *queue.Item
	pqOut     <-chan *url.URL
	pqErr     <-chan error

	retry time.Duration // duration between retry
	once  sync.Once     // used for closing Out
	done  chan struct{}
}

func (cw *Crawler) newScheduler(wq queue.WaitQueue) *scheduler {
	nworker := cw.opt.NWorker.Scheduler
	chIn, chOut, chErr := wq.Channel()
	this := &scheduler{
		NewIn:     make(chan *url.URL, nworker),
		DoneIn:    make(chan *url.URL, nworker),
		ErrIn:     make(chan *url.URL, nworker),
		RecoverIn: make(chan *url.URL, nworker),
		Out:       make(chan *url.URL, 4*nworker),
		cw:        cw,

		prioQueue: wq,
		pqIn:      chIn,
		pqOut:     chOut,
		pqErr:     chErr,

		retry: cw.opt.RetryDuration,
		done:  make(chan struct{}),
	}

	cw.initWorker(this, nworker)
	return this
}

func (sd *scheduler) work() {
	var (
		queueIn   chan<- *queue.Item
		output    chan<- *url.URL
		u, outURL *url.URL
		waiting   = make([]*queue.Item, 0, LinkPerPage)
		next      *queue.Item
		outURLs   []*url.URL
	)
	for {
		if len(outURLs) != 0 {
			output = sd.Out
			outURL = outURLs[0]
		}
		if len(waiting) > 0 {
			queueIn = sd.pqIn
			next = waiting[0]
		}
		select {
		// Input:
		case u = <-sd.NewIn:
			item, _ := sd.schedURL(u, URLTypeSeed, nil)
			waiting = append(waiting, item)
			continue
		case u = <-sd.RecoverIn:
			item, done := sd.schedURL(u, URLTypeRecover, nil)
			if !done {
				waiting = append(waiting, item)
				continue
			}
		case resp := <-sd.ResIn:
			sd.cw.store.IncVisitCount()
			for _, link := range resp.links {
				item, done := sd.schedURL(link.URL, URLTypeNew, resp)
				if !done {
					waiting = append(waiting, item)
				}
			}
			item, done := sd.schedURL(resp.URL, URLTypeResponse, resp)
			if !done {
				waiting = append(waiting, item)
				continue
			}
		case u = <-sd.DoneIn:
			sd.cw.store.UpdateStatus(u, URLStatusFinished)
		case u = <-sd.ErrIn:
			if cnt := sd.incErrCount(u); cnt >= sd.cw.opt.MaxRetry {
				sd.cw.store.UpdateStatus(u, URLStatusError)
				break
			}
			item := queue.NewItem()
			item.URL = u
			item.Next = time.Now().Add(sd.retry)
			waiting = append(waiting, item)
			continue
		case u = <-sd.pqOut:
			if u == nil { // queue has been closed
				return
			}
			outURLs = append(outURLs, u)

		// Output:
		case queueIn <- next:
			if waiting = waiting[1:]; len(waiting) == 0 {
				queueIn = nil
			}
		case output <- outURL:
			if outURLs = outURLs[1:]; len(outURLs) == 0 {
				output = nil
			}

		// Control:
		case err := <-sd.pqErr:
			if err != nil {
				log.Errorf("scheduler: wait queue error: %v", err)
			}
			return
		case <-sd.done:
			return
		case <-sd.quit:
			return
		}

		if is, err := sd.cw.store.IsFinished(); err != nil {
			log.Errorf("scheduler: %v", err)
			return
		} else if is {
			sd.stop() // notify other goroutines to exit.
			return
		}
	}
}

func (sd *scheduler) cleanup() {
	close(sd.Out)
	close(sd.pqIn)
	if err := sd.prioQueue.Close(); err != nil {
		log.Errorf("scheduler: close queue: %v", err)
	}
}

func (sd *scheduler) stop() {
	sd.once.Do(func() { close(sd.done) })
}

func (sd *scheduler) schedURL(u *url.URL, typ int, r *Response) (item *queue.Item, done bool) {
	uu, err := sd.cw.store.Get(u)
	if err != nil {
		// TODO
		return
	}
	switch typ {
	case URLTypeResponse:
		uu.VisitCount++
		uu.Last = r.Timestamp
		sd.cw.store.Update(uu)
	}

	minTime := uu.Last.Add(sd.cw.opt.MinDelay)
	item = queue.NewItem()
	item.URL = u
	if done, item.Next, item.Score = sd.cw.ctrl.Schedule(uu, typ, nil); done {
		sd.cw.store.UpdateStatus(u, URLStatusFinished)
		return
	}
	if item.Next.Before(minTime) {
		item.Next = minTime
	}
	return
}

func (sd *scheduler) incErrCount(u *url.URL) int {
	uu, _ := sd.cw.store.Get(u)
	cnt := uu.ErrorCount
	uu.ErrorCount++
	sd.cw.store.Update(uu)
	return cnt
}
