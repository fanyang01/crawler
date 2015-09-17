package crawler

import (
	"net/url"
	"sync"
	"time"
)

type filter struct {
	In     chan *Doc
	Out    chan *URL
	option *Option
	sites  sites
}

type sites struct {
	m map[string]*Site
	sync.RWMutex
}

type Scorer interface {
	Score(*URL) int64
}

func newFilter(opt *Option) *filter {
	ft := &filter{
		Out:    make(chan *URL, opt.URLFilter.OutQueueLen),
		option: opt,
	}
	ft.sites.m = make(map[string]*Site)
	return ft
}

func (ft *filter) Start(scorer Scorer) {
	go func() {
		for doc := range ft.In {
			uu, ok := ft.sites.getURL(doc.Loc)
			if !ok {
				// Redirect!!!
				// log.Println("shouldn't get here")
				uu = new(URL)
				uu.Loc = removeFragment(doc.Loc)
			}
			uu.Visited.Count++
			uu.Visited.Time = doc.Time
			ft.sites.addURLs(uu)

			for _, u := range doc.SubURLs {
				if !ft.testRobot(u) {
					continue
				}
				uu, ok := ft.sites.getURL(u)
				if !ok {
					uu = new(URL)
					uu.Loc = removeFragment(u)
				}

				uu.Score = scorer.Score(uu)
				if uu.Score <= 0 {
					uu.Score = 0
					continue
				}
				if uu.Score >= 1024 {
					uu.Score = 1024
				}
				uu.Priority = float64(uu.Score) / float64(1024)

				ft.Out <- uu
				uu.Enqueue.Count++
				uu.Enqueue.Time = time.Now()
				ft.sites.addURLs(uu)
			}
		}
		close(ft.Out)
	}()
}

func (st sites) addURLs(uu ...*URL) error {
	st.Lock()
	defer st.Unlock()
	for _, u := range uu {
		root := siteRoot(u.Loc)
		site, ok := st.m[root]
		if ok {
			site.URLs.Add(u)
			continue
		}

		if site, err := NewSiteFromURL(u.Loc); err != nil {
			return err
		} else {
			site.URLs.Add(u)
			st.m[site.Root] = site
		}
	}
	return nil
}

func (st sites) getURL(u *url.URL) (uu *URL, ok bool) {
	root := siteRoot(u)
	st.RLock()
	defer st.RUnlock()
	site, ok := st.m[root]
	if !ok {
		return
	}
	uu, ok = site.URLs.Get(u.RequestURI())
	return
}

func siteRoot(u *url.URL) string {
	uu := url.URL{
		Scheme: u.Scheme,
		Host:   u.Host,
	}
	return uu.String()
}

func (ft *filter) testRobot(u *url.URL) bool {
	root := siteRoot(u)
	ft.sites.RLock()
	defer ft.sites.RUnlock()
	site, ok := ft.sites.m[root]
	if !ok {
		return false
	}
	return site.Robot.TestAgent(u.Path, ft.option.RobotoAgent)
}

func removeFragment(u *url.URL) *url.URL {
	uu := *u
	u.Fragment = ""
	return &uu
}
