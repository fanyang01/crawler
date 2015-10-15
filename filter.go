package crawler

import (
	"fmt"
	"net/url"
	"sync"
)

type filter struct {
	In      chan *Link
	New     chan *url.URL
	Fetched chan *url.URL
	Done    chan struct{}
	nworker int
	handler Handler
	store   URLStore
	sites   *sites
}

func newFilter(nworker int, in chan *Link, done chan struct{},
	handler Handler, store URLStore) *filter {

	return &filter{
		New:     make(chan *url.URL, nworker),
		Fetched: make(chan *url.URL, nworker),
		Done:    done,
		In:      in,
		nworker: nworker,
		handler: handler,
		store:   store,
		sites:   newSites(),
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
		select {
		case ft.Fetched <- link.Base:
		case <-ft.Done:
			return
		}
		for _, anchor := range link.Anchors {
			if ft.handler.Accept(anchor) {
				// only handle new link
				if _, ok := ft.store.Get(*anchor.URL); ok {
					continue
				}
				if err := ft.addSite(anchor.URL); err != nil {
					// log
					continue
				}
				if ok := ft.testRobot(anchor.URL); !ok {
					continue
				}
				ft.store.Put(URL{
					Loc:    *anchor.URL,
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

func (ft *filter) addSite(u *url.URL) error {
	root := siteRoot(u)
	ft.sites.Lock()
	defer ft.sites.Unlock()

	site, ok := ft.sites.m[root]
	if ok {
		return nil
	}

	var err error
	site, err = newSite(root)
	if err != nil {
		return err
	}
	if err := site.fetchRobots(); err != nil {
		return fmt.Errorf("fetch robots.txt: %v", err)
	}
	site.fetchSitemap()
	for _, u := range site.Map.URLSet {
		uu := u.Loc
		if _, ok := ft.store.Get(uu); ok {
			continue
		}
		ft.store.Put(URL{
			Loc:          u.Loc,
			LastModified: u.LastModified,
			Score:        int64(u.Priority * 1024.0),
			Freq:         u.ChangeFreq,
		})
		ft.New <- &uu
	}
	ft.sites.m[root] = site
	return nil
}

func (ft *filter) testRobot(u *url.URL) bool {
	ft.sites.RLock()
	defer ft.sites.RUnlock()

	site, ok := ft.sites.m[siteRoot(u)]
	if !ok || site.Robot == nil {
		return false
	}
	return site.Robot.TestAgent(u.Path, RobotAgent)
}
