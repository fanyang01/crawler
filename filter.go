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
	In    chan *Link
	Out   chan *url.URL
	Done  chan struct{}
	Req   chan filterQuery
	store URLStore
}

type Sifter interface {
	Accept(Anchor) bool
}

func newFilter(nworker int, in chan *Link, done chan struct{},
	req chan schedQuery, store URLStore) *filter {
	return &filter{
		Out:   make(chan URL, nworker),
		Done:  done,
		In:    in,
		Req:   req,
		store: store,
	}
}

func (ft *filter) start() {
	var wg sync.WaitGroup
	wg.Add(nworker)
	for i := 0; i < nworker; i++ {
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
	for link := range ft.In {
		query := filterQuery{
			url:   link.Base,
			reply: make(chan Sifter),
		}
		ft.Req <- query
		sifter := <-query.reply
		for _, anchor := range link.Anchors {
			if sifter.Accept(anchor) {
				// only handle new link
				if _, ok := ft.store.Get(anchor.URL); ok {
					continue
				}
				ft.store.Put(URL{
					Loc:    *anchor.URL,
					Status: U_Init,
				})
				select {
				case ft.Out <- anchor.URL:
				case <-ft.Done:
					return
				}
			}
		}
	}
}
