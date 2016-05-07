package crawler

import (
	"net/url"
	"sync"
	"time"

	"golang.org/x/net/context"

	"github.com/fanyang01/crawler/queue"
)

type scheduler struct {
	workerConn
	cw *Crawler

	NewIn     chan *url.URL
	RecoverIn chan *url.URL
	RetryIn   chan *url.URL
	ErrIn     chan *url.URL
	Out       chan *url.URL
	In        chan *Response

	queue    queue.WaitQueue
	queueIn  chan<- *queue.Item
	queueOut <-chan *url.URL
	queueErr <-chan error

	once sync.Once // used for closing Out
	stop chan struct{}
}

func (cw *Crawler) newScheduler(wq queue.WaitQueue) *scheduler {
	nworker := cw.opt.NWorker.Scheduler
	queueIn, queueOut, queueErr := wq.Channel()

	this := &scheduler{
		cw: cw,

		NewIn:     make(chan *url.URL, nworker),
		RecoverIn: make(chan *url.URL, nworker),
		RetryIn:   make(chan *url.URL, nworker),
		ErrIn:     make(chan *url.URL, nworker),
		Out:       make(chan *url.URL, 4*nworker),

		queue:    wq,
		queueIn:  queueIn,
		queueOut: queueOut,
		queueErr: queueErr,

		stop: make(chan struct{}),
	}

	cw.initWorker("scheduler", this, nworker)
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
			queueIn = sd.queueIn
			next = waiting[0]
		}
		var (
			item     *queue.Item
			done, ok bool
			err      error
		)
		select {
		// Input:
		case u = <-sd.NewIn:
			item = sd.schedNewURL(nil, u)
			waiting = append(waiting, item)
			continue
		case u = <-sd.RecoverIn:
			item = sd.schedNewURL(nil, u)
			waiting = append(waiting, item)
			continue
		case resp, ok := <-sd.In:
			if !ok {
				sd.exit()
				return // closed
			}
			sd.cw.store.IncVisitCount()
			for _, link := range resp.links {
				item = sd.schedNewURL(resp, link.URL)
				waiting = append(waiting, item)
			}
			item, done, err = sd.schedResponse(resp)
			resp.Free()
			if err != nil {
				goto ERROR
			} else if !done {
				waiting = append(waiting, item)
				continue
			}
		case u = <-sd.RetryIn:
			if item, ok, err = sd.retry(u); err != nil {
				goto ERROR
			} else if ok {
				waiting = append(waiting, item)
				continue
			}
		case u = <-sd.ErrIn:
			if err = sd.cw.store.UpdateStatus(u, URLStatusError); err != nil {
				goto ERROR
			}
		case u = <-sd.queueOut:
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
		case err = <-sd.queueErr:
			if err != nil {
				goto ERROR
			}
			return
		case <-sd.stop:
			return
		case <-sd.quit:
			return
		}

		if ok, err = sd.cw.store.IsFinished(); err != nil {
			goto ERROR
		} else if ok {
			sd.exit()
			return
		}
		continue

	ERROR:
		sd.logger.Error("scheduler error", "err", err)
		sd.exit() // notify other scheduler goroutines to exit.
		return
	}
}

func (sd *scheduler) cleanup() {
	close(sd.Out)
	close(sd.queueIn)
	if err := sd.queue.Close(); err != nil {
		sd.logger.Error("close wait queue", "err", err)
	}
}

func (sd *scheduler) exit() {
	sd.once.Do(func() { close(sd.stop) })
}

func (sd *scheduler) schedNewURL(r *Response, u *url.URL) *queue.Item {
	item := queue.NewItem()
	item.URL = u
	t := sd.cw.ctrl.Sched(r, u)
	if t.Ctx == nil {
		t.Ctx = context.TODO()
	}
	item.Next, item.Score, item.Ctx = t.At, t.Score, t.Ctx
	return item
}

func (sd *scheduler) schedResponse(r *Response) (
	item *queue.Item, done bool, err error,
) {
	uu, err := sd.cw.store.Get(r.URL)
	if err != nil {
		return
	}
	uu.VisitCount++
	uu.Last = r.Timestamp
	if err = sd.cw.store.Update(uu); err != nil {
		return
	}

	minTime := uu.Last.Add(sd.cw.opt.MinDelay)
	item = queue.NewItem()
	item.URL = r.URL

	var t Ticket
	done, t = sd.cw.ctrl.Resched(r)
	if done {
		err = sd.cw.store.UpdateStatus(r.URL, URLStatusFinished)
		return
	} else if t.Ctx == nil {
		t.Ctx = context.TODO()
	}
	item.Next, item.Score, item.Ctx = t.At, t.Score, t.Ctx
	if item.Next.Before(minTime) {
		item.Next = minTime
	}
	return
}

func (sd *scheduler) retry(u *url.URL) (*queue.Item, bool, error) {
	if cnt, err := sd.incErrCount(u); err != nil {
		return nil, false, err
	} else if cnt >= sd.cw.opt.MaxRetry {
		err := sd.cw.store.UpdateStatus(u, URLStatusError)
		return nil, false, err
	}
	item := queue.NewItem()
	item.URL = u
	item.Next = time.Now().Add(sd.cw.opt.RetryDuration)
	return item, true, nil
}

func (sd *scheduler) incErrCount(u *url.URL) (cnt int, err error) {
	var uu *URL
	if uu, err = sd.cw.store.Get(u); err != nil {
		return
	}
	uu.ErrorCount++
	cnt = uu.ErrorCount
	err = sd.cw.store.Update(uu)
	return
}
