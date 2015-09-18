package crawler

import (
	"log"
	"net/url"
	"sync"
	"time"
)

type filter struct {
	In        chan *Doc
	Out       chan *URL
	option    *Option
	poolMutex sync.Mutex
	pool      *pool
	workers   chan struct{}
	scorer    Scorer
	crawler   *Crawler
}

type Scorer interface {
	Score(*URL) int64
}

func newFilter(cw *Crawler, opt *Option) *filter {
	ft := &filter{
		Out:     make(chan *URL, opt.URLFilter.OutQueueLen),
		option:  opt,
		pool:    newPool(),
		crawler: cw,
	}
	return ft
}

func (ft *filter) visit(doc *Doc) {
	ft.poolMutex.Lock()
	uu, ok := ft.pool.Get(doc.Loc)
	if !ok {
		// Redirect!!!
		uu = newURL(doc.Loc)
	}
	uu.Visited.Count++
	uu.Visited.Time = doc.Time
	ft.pool.Add(uu)
	ft.poolMutex.Unlock()
}

func (ft *filter) handleSubURL(u *url.URL) {
	ft.poolMutex.Lock()
	defer ft.poolMutex.Unlock()

	uu, ok := ft.pool.Get(*u)
	if !ok {
		uu = newURL(*u)
	}

	uu.Score = ft.scorer.Score(uu)
	if uu.Score <= 0 {
		uu.Score = 0
	}
	if uu.Score >= 1024 {
		uu.Score = 1024
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

	ft.Out <- uu
	uu.Enqueue.Count++
	uu.Enqueue.Time = time.Now()
	ft.pool.Add(uu)
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
			go func() {
				ft.visit(doc)

				for _, u := range doc.SubURLs {
					ft.handleSubURL(u)
				}
				ft.workers <- struct{}{}
			}()
		}
		close(ft.Out)
	}()
}
