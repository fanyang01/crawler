package crawler

import (
	"net/url"
	"sync"
	"time"

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
			item, _, err = sd.schedURL(nil, u, URLTypeSeed)
			if err != nil {
				goto ERROR
			}
			waiting = append(waiting, item)
			continue
		case u = <-sd.RecoverIn:
			item, done, err = sd.schedURL(nil, u, URLTypeRecover)
			if err != nil {
				goto ERROR
			} else if !done {
				waiting = append(waiting, item)
				continue
			}
		case resp, ok := <-sd.In:
			if !ok {
				sd.exit()
				return // closed
			}
			sd.cw.store.IncVisitCount()
			for _, link := range resp.links {
				item, done, err = sd.schedURL(resp, link.URL, URLTypeNew)
				if err != nil {
					goto ERROR
				} else if !done {
					waiting = append(waiting, item)
				}
			}
			item, done, err = sd.schedURL(resp, resp.URL, URLTypeResponse)
			resp.Free()
			if err != nil {
				goto ERROR
			} else if !done {
				waiting = append(waiting, item)
				continue
			}
		case u = <-sd.RetryIn:
			if item, ok = sd.retry(u); ok {
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

func (sd *scheduler) schedURL(r *Response, u *url.URL, typ int) (item *queue.Item, done bool, err error) {
	uu, err := sd.cw.store.Get(u)
	if err != nil {
		// TODO
		return
	}
	var ctx *Context
	switch typ {
	case URLTypeResponse:
		uu.VisitCount++
		uu.Last = r.Timestamp
		if err = sd.cw.store.Update(uu); err != nil {
			return
		}
		ctx = r.Context()
	case URLTypeNew:
		ctx = newContext(sd.cw, u)
		ctx.response = r
	default:
		ctx = newContext(sd.cw, u)
	}
	ctx.fromURL(uu)

	minTime := uu.Last.Add(sd.cw.opt.MinDelay)
	item = queue.NewItem()
	item.URL = u
	done, item.Next, item.Score = sd.cw.ctrl.Schedule(ctx, u)
	ctx.response = nil
	if done {
		sd.cw.store.UpdateStatus(u, URLStatusFinished)
		return
	}
	if item.Next.Before(minTime) {
		item.Next = minTime
	}
	return
}

func (sd *scheduler) retry(u *url.URL) (*queue.Item, bool) {
	if cnt := sd.incErrCount(u); cnt >= sd.cw.opt.MaxRetry {
		sd.cw.store.UpdateStatus(u, URLStatusError)
		return nil, false
	}
	item := queue.NewItem()
	item.URL = u
	item.Next = time.Now().Add(sd.cw.opt.RetryDuration)
	return item, true
}

func (sd *scheduler) incErrCount(u *url.URL) int {
	uu, _ := sd.cw.store.Get(u)
	cnt := uu.ErrorCount
	uu.ErrorCount++
	sd.cw.store.Update(uu)
	return cnt
}
