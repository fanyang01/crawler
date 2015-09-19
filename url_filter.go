package crawler

import (
	"log"
	"net/url"
	"time"
)

type filter struct {
	In      chan *Doc
	Out     chan *URL
	option  *Option
	workers chan struct{}
	scorer  Scorer
	crawler *Crawler
}

type Scorer interface {
	Score(*URL) (score int64, at time.Time)
}

func newFilter(cw *Crawler, opt *Option) *filter {
	ft := &filter{
		Out:     make(chan *URL, opt.URLFilter.OutQueueLen),
		option:  opt,
		crawler: cw,
	}
	return ft
}

func (ft *filter) visit(doc *Doc) {
	ft.crawler.pool.Lock()
	uu, ok := ft.crawler.pool.Get(doc.Loc)
	if !ok {
		// Redirect!!!
		uu = newURL(doc.Loc)
	}
	uu.processing = false
	uu.Visited.Count++
	uu.Visited.Time = doc.Time
	ft.crawler.pool.Add(uu)
	ft.crawler.pool.Unlock()
}

func (ft *filter) handleSubURL(u *url.URL) {
	ft.crawler.pool.Lock()
	defer ft.crawler.pool.Unlock()

	uu, ok := ft.crawler.pool.Get(*u)
	if !ok {
		uu = newURL(*u)
	}
	if uu.processing {
		return
	}

	uu.Score, uu.nextTime = ft.scorer.Score(uu)
	if uu.Score <= 0 {
		uu.Score = 0
	}
	if uu.Score >= 1024 {
		uu.Score = 1024
	}
	minTime := uu.Visited.Time.Add(ft.option.MinDelay)
	if uu.Visited.Count > 0 && uu.nextTime.Before(minTime) {
		uu.nextTime = minTime
	}

	if uu.Score == 0 {
		return
	}
	if err := ft.crawler.addSite(u); err != nil {
		log.Println(err)
		return
	}
	if accept := ft.crawler.testRobot(u); !accept {
		return
	}

	uu.Priority = float64(uu.Score) / float64(1024)
	uu.processing = true
	ft.crawler.pool.Add(uu)
	ft.Out <- uu
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
				ft.visit(d)

				for _, u := range d.SubURLs {
					ft.handleSubURL(u)
				}
				ft.workers <- struct{}{}
			}(doc)
		}
		close(ft.Out)
	}()
}
