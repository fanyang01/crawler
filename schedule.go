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
	ErrIn     chan *Context
	Out       chan *Context
	In        chan *Response
	ErrRespIn chan *Response

	queue    queue.WaitQueue
	queueIn  chan<- *queue.Item
	queueOut <-chan *queue.Item
	queueErr <-chan error

	stop chan struct{}
	once sync.Once // used for closing Out
}

func (cw *Crawler) newScheduler(wq queue.WaitQueue) *scheduler {
	nworker := cw.opt.NWorker.Scheduler
	queueIn, queueOut, queueErr := wq.Channel()

	this := &scheduler{
		cw: cw,

		NewIn:     make(chan *url.URL, nworker),
		RecoverIn: make(chan *url.URL, nworker),
		ErrIn:     make(chan *Context, nworker),
		ErrRespIn: make(chan *Response, nworker),
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
		case u := <-sd.NewIn:
			item = sd.sched(nil, u)
			waiting = append(waiting, item)
			continue
		case u := <-sd.RecoverIn:
			item = sd.sched(nil, u)
			waiting = append(waiting, item)
			continue
		case ctx := <-sd.ErrIn:
			switch ctx.err.(type) {
			case FatalError, *FatalError:
				err = ctx.err
				goto ERROR
			case RetryableError, *RetryableError:
				if item, ok, err = sd.retry(ctx); err != nil {
					goto ERROR
				} else if ok {
					waiting = append(waiting, item)
					continue
				}
			default:
				if err = sd.cw.store.Complete(ctx.url); err != nil {
					goto ERROR
				}
				sd.logger.Error(
					"complete due to unknown error",
					"err", ctx.err, "url", ctx.url,
				)
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
			if err != nil {
				goto ERROR
			} else if !done {
				waiting = append(waiting, item)
				continue
			}
		case resp := <-sd.ErrRespIn:
			// NOTE: even if an error occured, links found in the response
			// should still be enqueued, because the state of storage has
			// been changed.
			for _, url := range resp.links {
				item = sd.sched(resp, url)
				waiting = append(waiting, item)
			}
			switch resp.ctx.err.(type) {
			case FatalError, *FatalError:
				err = resp.ctx.err
				goto ERROR
			case RetryableError, *RetryableError:
				if item, ok, err = sd.retry(resp.ctx); err != nil {
					goto ERROR
				} else if ok {
					waiting = append(waiting, item)
					continue
				}
			default:
				sd.logger.Error(
					"complete due to unknown error",
					"err", resp.ctx.err, "url", resp.URL,
				)
				if err = sd.cw.store.Complete(resp.URL); err != nil {
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
		sd.logger.Crit("unrecoverable error, exiting...", "err", err)
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
	select {
	case <-sd.quit: // closed
	default:
		close(sd.quit)
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
		t.Ctx = context.Background()
	}
	item.Next, item.Score, item.Ctx = t.At, t.Score, t.Ctx
	return item
}

func (sd *scheduler) resched(r *Response) (
	item *queue.Item, done bool, err error,
) {
	defer func() {
		r.ctx.free()
		r.free()
	}()

	var last time.Time
	if err = sd.cw.store.UpdateFunc(r.URL, func(u *URL) {
		u.NumVisit++
		u.NumRetry = 0
		last = u.Last
		u.Last = r.Timestamp
	}); err != nil {
		return
	}

	var t Ticket
	done, t = sd.cw.ctrl.Resched(r)
	if done {
		err = sd.cw.store.Complete(r.URL)
		return
	} else if t.Ctx == nil {
		t.Ctx = context.Background()
	}

	item = queue.NewItem()
	item.URL = r.URL
	item.Next, item.Score, item.Ctx = t.At, t.Score, t.Ctx
	min := last.Add(sd.cw.opt.MinDelay)
	if item.Next.Before(min) {
		item.Next = min
	}
	return
}

func (sd *scheduler) retry(ctx *Context) (*queue.Item, bool, error) {
	defer ctx.free()

	var cnt int
	if err := sd.cw.store.UpdateFunc(ctx.url, func(uu *URL) {
		uu.NumRetry++
		cnt = uu.NumRetry
	}); err != nil {
		return nil, false, err
	}

	delay, max := sd.cw.ctrl.Retry(ctx)
	if cnt >= max {
		sd.logger.Error(
			"exceed maximum number of retries",
			"err", ctx.err, "url", ctx.url, "retries", cnt,
		)
		err := sd.cw.store.Complete(ctx.url)
		return nil, false, err
	}

	sd.logger.Error(
		"retry due to error",
		"err", ctx.err, "url", ctx.url, "retries", cnt,
	)
	item := queue.NewItem()
	item.URL = ctx.url
	item.Ctx = ctx.C
	item.Next = time.Now().Add(delay)
	return item, true, nil
}
