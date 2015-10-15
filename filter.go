package crawler

import (
	"fmt"
	"log"
	"net/url"
	"sync"
)

type filter struct {
	In       chan *Link
	NewOut   chan *url.URL
	AgainOut chan *url.URL
	Done     chan struct{}
	WG       *sync.WaitGroup
	nworker  int
	handler  Handler
	store    URLStore
	sites    *sites
}

func newFilter(nworker int, handler Handler, store URLStore) *filter {

	return &filter{
		NewOut:   make(chan *url.URL, nworker),
		AgainOut: make(chan *url.URL, nworker),
		nworker:  nworker,
		handler:  handler,
		store:    store,
		sites:    newSites(),
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
		close(ft.NewOut)
		close(ft.AgainOut)
		ft.WG.Done()
	}()
}

func (ft *filter) work() {
	for link := range ft.In {
		select {
		case ft.AgainOut <- link.Base:
		case <-ft.Done:
			return
		}
		depth := 0
		if base, ok := ft.store.Get(*link.Base); ok {
			depth = base.Depth + 1
		}
		for _, anchor := range link.Anchors {
			anchor.Depth = depth
			if ft.handler.Accept(anchor) {
				// only handle new link
				if _, ok := ft.store.Get(*anchor.URL); ok {
					continue
				}
				if err := ft.addSite(anchor.URL); err != nil {
					log.Printf("add site: %v", err)
					continue
				}
				if ok := ft.testRobot(anchor.URL); !ok {
					continue
				}
				ft.store.Put(URL{
					Loc:    *anchor.URL,
					Status: U_Init,
					Depth:  anchor.Depth,
				})
				select {
				case ft.NewOut <- anchor.URL:
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
		ft.NewOut <- &uu
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
