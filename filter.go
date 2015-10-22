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
	In     chan *Link
	Out    chan *Link
	NewOut chan *url.URL
	ctrler Controller
	store  URLStore
	sites  *sites
}

func (cw *Crawler) newFilter() *filter {
	nworker := cw.opt.NWorker.Filter
	this := &filter{
		Out:    make(chan *Link, nworker),
		ctrler: cw.ctrler,
		store:  cw.urlStore,
		sites:  newSites(),
	}
	this.nworker = nworker
	this.wg = &cw.wg
	this.quit = cw.quit
	return this
}

func (ft *filter) cleanup() {
	close(ft.Out)
}

func (ft *filter) work() {
	for link := range ft.In {
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
				if ft.store.PutIfNonExist(&URL{
					Loc:   *anchor.URL,
					Depth: anchor.Depth,
				}) {
					anchor.ok = true
				}
			}
		}
		select {
		case ft.Out <- link:
		case <-ft.quit:
			return
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
		if ft.store.PutIfNonExist(&URL{
			Loc:     u.Loc,
			LastMod: u.LastModified,
			Freq:    u.ChangeFreq,
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
