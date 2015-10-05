package crawler

import (
	"log"
	"net/url"
	"sync"
	"time"
)

type filter struct {
	In        chan *Doc
	Out       chan URL
	Done      chan struct{}
	option    *Option
	scheduler Scheduler
	cw        *Crawler
}

type Scheduler interface {
	Schedule(URL) (score int64, at time.Time)
	Accept(url.URL) bool
}

func newFilter(opt *Option, cw *Crawler, scheduler Scheduler) *filter {
	ft := &filter{
		Out:       make(chan URL, opt.URLFilter.QLen),
		option:    opt,
		cw:        cw,
		scheduler: scheduler,
	}
	return ft
}

func (ft *filter) Start() {
	n := ft.option.URLFilter.NWorker
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			ft.work()
			wg.Done()
		}()
	}
	go func() {
		wg.Wait()
		close(ft.Out)
	}()
}

func (ft *filter) work() {
	for d := range ft.In {
		ft.handleDocURL(d)
		for _, u := range d.SubURLs {
			ft.handleSubURL(*u)
		}
		select {
		case <-ft.Done:
			return
		default:
		}
	}
}

func (ft *filter) handleDocURL(doc *Doc) {
	f := func(entry *storeEntry) {
		uu := &entry.url
		uu.Visited.Count++
		uu.Visited.Time = doc.Time
		uu.LastModified = doc.LastModified
		uu.Depth = doc.Depth + 1
		ft.enqueue(uu)
		entry.Unlock()
	}

	entry := ft.cw.pool.Get(doc.URL)
	f(entry)

	if doc.Loc.String() != doc.requestURL.String() {
		entry = ft.cw.pool.Get(*newURL(*doc.requestURL))
		f(entry)
	}
}

func (ft *filter) handleSubURL(u url.URL) {
	if !ft.scheduler.Accept(u) {
		return
	}
	entry := ft.cw.pool.Get(*newURL(u))
	if !entry.url.processing {
		ft.enqueue(&entry.url)
	}
	entry.Unlock()
}

func (ft *filter) enqueue(uu *URL) {
	if keep := ft.schedule(uu); !keep {
		uu.processing = false
		return
	}

	if err := ft.cw.addSite(uu.Loc); err != nil {
		log.Printf("add site: %v\n", err)
		uu.processing = false
		return
	}
	if accept := ft.cw.testRobot(uu.Loc); !accept {
		uu.processing = false
		return
	}

	uu.processing = true
	select {
	case ft.Out <- *uu:
	case <-ft.Done:
		return
	}
}

func (ft *filter) schedule(uu *URL) (keep bool) {
	uu.Score, uu.nextTime = ft.scheduler.Schedule(*uu)
	if uu.Score <= 0 {
		return
	}
	if uu.Score >= 1024 {
		uu.Score = 1024
	}
	minTime := uu.Visited.Time.Add(ft.option.MinDelay)
	if uu.Visited.Count > 0 && uu.nextTime.Before(minTime) {
		uu.nextTime = minTime
	}
	uu.Priority = float64(uu.Score) / float64(1024)
	return true
}
