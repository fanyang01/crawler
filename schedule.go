package crawler

import (
	"net/url"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
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
	pq := cw.NewMemQueue(PQueueLen)
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

	cw.initWorker(this, nworker)
	return this
}

func (sd *scheduler) cleanup() { close(sd.Out) }

func (sd *scheduler) work() {
	var (
		queueIn   chan<- *SchedItem
		output    chan<- *url.URL
		u, outURL *url.URL
		waiting   = make([]*SchedItem, 0, LinkPerPage)
		next      *SchedItem
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
			sd.cw.store.UpdateStatus(u, URLfinished)
		case u = <-sd.ErrIn:
			if cnt := sd.incErrCount(u); cnt >= sd.cw.opt.MaxRetry {
				sd.cw.store.UpdateStatus(u, URLerror)
				break
			}
			waiting = append(waiting, &SchedItem{
				URL:  u,
				Next: time.Now().Add(sd.retry),
			})
			continue
		case item := <-sd.pqOut:
			outURLs = append(outURLs, item.URL)

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
		case <-sd.done:
			return
		case <-sd.quit:
			sd.once.Do(func() {
				sd.prioQueue.Close()
			})
			return
		}
		if is, err := sd.cw.store.IsFinished(); err != nil {
			logrus.Error(err)
			return
		} else if is {
			sd.stop()
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

func (sd *scheduler) schedURL(u *url.URL, typ int, r *Response) (item *SchedItem, done bool) {
	uu, err := sd.cw.store.Get(u)
	if err != nil {
		// TODO
		return
	}

	minTime := uu.LastTime.Add(sd.cw.opt.MinDelay)
	item = &SchedItem{
		URL: u,
	}
	if done, item.Next, item.Score = sd.cw.ctrl.Schedule(uu, typ, nil); done {
		sd.cw.store.UpdateStatus(u, URLfinished)
		return
	}
	if item.Next.Before(minTime) {
		item.Next = minTime
	}

	switch typ {
	case URLTypeResponse:
		uu.VisitCount++
		uu.LastTime = r.Timestamp
		uu.LastMod = r.LastModified
		sd.cw.store.Update(uu)
	}
	return
}

func (sd *scheduler) incErrCount(u *url.URL) int {
	uu, _ := sd.cw.store.Get(u)
	cnt := uu.ErrCount
	uu.ErrCount++
	sd.cw.store.Update(uu)
	return cnt
}
