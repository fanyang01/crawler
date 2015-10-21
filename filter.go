package crawler

import (
	"fmt"
	"log"
	"net/url"
)

var (
	RobotAgent = "gocrawler"
)

type filter struct {
	workerConn
	In       chan *Link
	NewOut   chan *url.URL
	AgainOut chan *url.URL
	ctrler   Controller
	store    URLStore
	sites    *sites
}

func (cw *Crawler) newFilter() *filter {
	nworker := cw.opt.NWorker.Filter
	this := &filter{
		NewOut:   make(chan *url.URL, nworker),
		AgainOut: make(chan *url.URL, nworker),
		ctrler:   cw.ctrler,
		store:    cw.urlStore,
		sites:    newSites(),
	}
	this.nworker = nworker
	this.WG = &cw.wg
	this.Done = cw.done
	return this
}

func (ft *filter) cleanup() {
	close(ft.NewOut)
	close(ft.AgainOut)
}

func (ft *filter) work() {
	for link := range ft.In {
		select {
		case ft.AgainOut <- link.Base:
		case <-ft.Done:
			return
		}
		depth := ft.store.GetDepth(link.Base)
		for _, anchor := range link.Anchors {
			anchor.Depth = depth + 1
			anchor.URL.Fragment = ""
			if ft.ctrler.Accept(anchor) {
				// only handle new link
				if ft.store.Exist(anchor.URL) {
					continue
				}
				if err := ft.addSite(anchor.URL); err != nil {
					log.Printf("add site: %v", err)
					continue
				}
				if ok := ft.testRobot(anchor.URL); !ok {
					continue
				}
				ft.store.PutIfNonExist(&URL{
					Loc:   *anchor.URL,
					Depth: anchor.Depth,
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
		if ft.store.Exist(&u.Loc) {
			continue
		}
		if ft.store.PutIfNonExist(&URL{
			Loc:          u.Loc,
			LastModified: u.LastModified,
			Score:        int64(u.Priority * 1024.0),
			Freq:         u.ChangeFreq,
		}) {
			loc := u.Loc
			ft.NewOut <- &loc
		}
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
