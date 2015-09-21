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

func (ft *filter) Visit(uu *URL) {
	if keep := ft.visit(uu); !keep {
		uu.processing = false
		ft.crawler.pool.Add(uu)
		return
	}
	uu.processing = true
	ft.crawler.pool.Add(uu)
	ft.Out <- uu
}

func (ft *filter) visit(uu *URL) (keep bool) {
	uu.Score, uu.nextTime = ft.scorer.Score(uu)
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

	if err := ft.crawler.addSite(&uu.Loc); err != nil {
		log.Println(err)
		return
	}
	if accept := ft.crawler.testRobot(&uu.Loc); !accept {
		return
	}

	uu.Priority = float64(uu.Score) / float64(1024)
	return true
}

func (ft *filter) VisitAgain(doc *Doc) {
	f := func(u url.URL, depth int) {
		uu, ok := ft.crawler.pool.Get(u)
		if !ok {
			// Redirect!!!
			uu = newURL(u)
		}
		uu.processing = true
		uu.Visited.Count++
		uu.Visited.Time = doc.Time
		uu.Depth = depth + 1
		ft.Visit(uu)
	}

	ft.crawler.pool.Lock()
	f(doc.Loc, doc.Depth)
	if doc.Loc.String() != doc.requestURL.String() {
		f(*doc.requestURL, doc.Depth)
	}
	ft.crawler.pool.Unlock()
}

func (ft *filter) VisitSubURL(u *url.URL) {
	ft.crawler.pool.Lock()
	defer ft.crawler.pool.Unlock()

	uu, ok := ft.crawler.pool.Get(*u)
	if !ok {
		uu = newURL(*u)
	}
	if uu.processing {
		return
	}
	ft.Visit(uu)
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
				for _, u := range d.SubURLs {
					ft.VisitSubURL(u)
				}
				ft.VisitAgain(d)
				ft.workers <- struct{}{}
			}(doc)
		}
		close(ft.Out)
	}()
}
