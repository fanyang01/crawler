package crawler

import (
	"net/url"
	"sync"
)

var (
	UrlBufSize = 64
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

type URLFilter interface {
	Filter(*URL) bool
}

func newFilter(opt *Option) *filter {
	ft := &filter{
		Out:    make(chan *URL, opt.URLFilter.OutQueueLen),
		option: opt,
	}
	ft.sites.m = make(map[string]*Site)
	return ft
}

func (ft *filter) Start(filters ...URLFilter) {
	go func() {
		for doc := range ft.In {
			for _, u := range doc.SubURLs {
				uu, ok := ft.sites.getURL(u)
				if !ok {
					uu = new(URL)
					uu.Loc = u
				}

				var accept = true
				for _, filter := range filters {
					if accept = filter.Filter(uu); !accept {
						break
					}
				}
				if accept {
					ft.Out <- uu
				}
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
