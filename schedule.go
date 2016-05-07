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
	ErrURLIn  chan *Context
	Out       chan *Context
	In        chan *Response
	ErrIn     chan *Response

	queue    queue.WaitQueue
	queueIn  chan<- *queue.Item
	queueOut <-chan *queue.Item
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
		ErrURLIn:  make(chan *Context, nworker),
		ErrIn:     make(chan *Response, nworker),
		Out:       make(chan *Context, nworker),

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
		queueIn chan<- *queue.Item
		waiting = make([]*queue.Item, 0, perPage)
		next    *queue.Item
		u       *url.URL
		out     chan<- *Context
		first   *Context
		outFIFO []*Context
	)
	for {
		if len(outFIFO) != 0 {
			out = sd.Out
			first = outFIFO[0]
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
			item = sd.sched(nil, u)
			waiting = append(waiting, item)
			continue
		case u = <-sd.RecoverIn:
			item = sd.sched(nil, u)
			waiting = append(waiting, item)
			continue
		case ctx := <-sd.ErrURLIn:
			if err = sd.cw.store.Complete(ctx.url); err != nil {
				goto ERROR
			}
		case u = <-sd.RetryIn:
			if item, ok, err = sd.retry(u); err != nil {
				goto ERROR
			} else if ok {
				waiting = append(waiting, item)
				continue
			}

		case resp, ok := <-sd.In:
			if !ok {
				sd.exit()
				return // closed
			}
			sd.cw.store.IncVisitCount()
			for _, url := range resp.links {
				item = sd.sched(resp, url)
				waiting = append(waiting, item)
			}
			item, done, err = sd.resched(resp)
			resp.Free()
			if err != nil {
				goto ERROR
			} else if !done {
				waiting = append(waiting, item)
				continue
			}
		case resp := <-sd.ErrIn:
			// NOTE: even if an error occured, links found in the response
			// should still be enqueued, because the state of storage has
			// been changed.
			for _, url := range resp.links {
				item = sd.sched(resp, url)
				waiting = append(waiting, item)
			}
			switch resp.err.(type) {
			case RetriableError:
				if item, ok, err = sd.retry(u); err != nil {
					goto ERROR
				} else if ok {
					waiting = append(waiting, item)
					continue
				}
			default:
				if err = sd.cw.store.Complete(u); err != nil {
					goto ERROR
				}
			}

		case item := <-sd.queueOut:
			if item == nil { // queue has been closed
				return
			}
			outFIFO = append(
				outFIFO, sd.cw.newContext(item.URL, item.Ctx),
			)
			item.Free()

		// Output:
		case queueIn <- next:
			if waiting = waiting[1:]; len(waiting) == 0 {
				queueIn = nil
			}
		case out <- first:
			if outFIFO = outFIFO[1:]; len(outFIFO) == 0 {
				out = nil
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

func (sd *scheduler) sched(r *Response, u *url.URL) *queue.Item {
	item := queue.NewItem()
	item.URL = u
	t := sd.cw.ctrl.Sched(r, u)
	if t.Ctx == nil {
		t.Ctx = context.TODO()
	}
	item.Next, item.Score, item.Ctx = t.At, t.Score, t.Ctx
	return item
}

func (sd *scheduler) resched(r *Response) (
	item *queue.Item, done bool, err error,
) {
	uu, err := sd.cw.store.Get(r.URL)
	if err != nil {
		return
	}
	uu.NumVisit++
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
		err = sd.cw.store.Complete(r.URL)
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
		err := sd.cw.store.Complete(u)
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
	uu.NumError++
	cnt = uu.NumError
	err = sd.cw.store.Update(uu)
	return
}
