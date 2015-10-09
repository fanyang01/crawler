package crawler

import (
	"net/url"
	"sync"
)

type filterQuery struct {
	url   *url.URL
	reply chan Sifter
}

type filter struct {
	In      chan *Link
	New     chan *url.URL
	Fetched chan *url.URL
	Done    chan struct{}
	Req     chan *ctrlQuery
	nworker int
	store   URLStore
}

type Sifter interface {
	Accept(Anchor) bool
}

func newFilter(nworker int, in chan *Link, done chan struct{},
	req chan *ctrlQuery, store URLStore) *filter {

	return &filter{
		New:     make(chan *url.URL, nworker),
		Fetched: make(chan *url.URL, nworker),
		Done:    done,
		In:      in,
		Req:     req,
		nworker: nworker,
		store:   store,
	}
}

func (ft *filter) start() {
	var wg sync.WaitGroup
	wg.Add(ft.nworker)
	for i := 0; i < ft.nworker; i++ {
		go func() {
			ft.work()
			wg.Done()
		}()
	}
	go func() {
		wg.Wait()
		close(ft.New)
		close(ft.Fetched)
	}()
}

func (ft *filter) work() {
	for link := range ft.In {
		query := &ctrlQuery{
			url:   link.Base,
			reply: make(chan Controller),
		}
		ft.Req <- query
		sifter := <-query.reply
		for _, anchor := range link.Anchors {
			if sifter.Accept(anchor) {
				// only handle new link
				if _, ok := ft.store.Get(*anchor.URL); ok {
					continue
				}
				ft.store.Put(URL{
					Loc:    anchor.URL,
					Status: U_Init,
				})
				select {
				case ft.New <- anchor.URL:
				case <-ft.Done:
					return
				}
			}
		}
	}
}
