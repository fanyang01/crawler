package crawler

import (
	"log"
	"net/url"
	"time"
)

type filter struct {
	In      chan *Doc
	Out     chan URL
	option  *Option
	workers chan struct{}
	scorer  Scorer
	cw      *Crawler
}

type Scorer interface {
	Score(URL) (score int64, at time.Time)
	Accept(url.URL) bool
}

func newFilter(cw *Crawler, opt *Option) *filter {
	ft := &filter{
		Out:    make(chan URL, opt.URLFilter.OutQueueLen),
		option: opt,
		cw:     cw,
	}
	return ft
}

func (ft *filter) Start(scorer Scorer) {
	ft.scorer = scorer
	ft.workers = make(chan struct{}, ft.option.URLFilter.NumOfWorkers)
	for i := 0; i < ft.option.URLFilter.NumOfWorkers; i++ {
		ft.workers <- struct{}{}
	}
	go func() {
		for doc := range ft.In {
			<-ft.workers
			go func(d *Doc) {
				ft.handleDocURL(d)
				for _, u := range d.SubURLs {
					ft.handleSubURL(*u)
				}
				ft.workers <- struct{}{}
			}(doc)
		}
		close(ft.Out)
	}()
}

func (ft *filter) handleDocURL(doc *Doc) {
	f := func(entry *poolEntry) {
		uu := &entry.url
		uu.Visited.Count++
		uu.Visited.Time = doc.Time
		uu.LastModified = doc.LastModified
		uu.Depth = doc.Depth + 1
		ft.do(uu)
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
	if !ft.scorer.Accept(u) {
		return
	}
	entry := ft.cw.pool.Get(*newURL(u))
	if !entry.url.processing {
		ft.do(&entry.url)
	}
	entry.Unlock()
}

func (ft *filter) do(uu *URL) {
	if keep := ft.score(uu); !keep {
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
	ft.Out <- *uu
}

func (ft *filter) score(uu *URL) (keep bool) {
	uu.Score, uu.nextTime = ft.scorer.Score(*uu)
	if uu.Score <= 0 {
		uu.Score = 0
	}
	if uu.Score == 0 {
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
