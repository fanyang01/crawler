package crawler

import (
	"errors"
	"net/url"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/fanyang01/crawler"
	"github.com/fanyang01/crawler/sitemap"
	robot "github.com/temoto/robotstxt-go"
)

var (
	ErrUnsupportedProtocol = errors.New("sitemeta: unsupported protocol")
	ErrNoHost              = errors.New("sitemeta: host can't be empty")
)

type sitemeta struct {
	sync.RWMutex

	robot    *robot.RobotsData
	rootURL  *url.URL
	sitemap  sitemap.Sitemap
	interval time.Duration
	waiting  int
	nextTime time.Time // the next time this site should be visited at

	visited struct {
		lastTime  time.Time
		sitemap   time.Time
		robotstxt time.Time
	}
}

func newSiteFromURL(u *url.URL) (*sitemeta, error) {
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, ErrUnsupportedProtocol
	}
	if u.Host == "" {
		return nil, ErrNoHost
	}
	uu := &url.URL{
		Scheme: u.Scheme,
		Host:   u.Host,
	}
	robots := *uu
	robots.Path = "/robots.txt"
	site := &sitemeta{
		rootURL: uu,
	}
	return site, nil
}

func (s *sitemeta) RobotsURL() string {
	return s.rootURL.String() + "/robots.txt"
}

func (s *sitemeta) sitemapURLs() (urls []string) {
	if s.robot == nil {
		urls = []string{s.rootURL.String() + "/sitemap.xml"}
	} else {
		urls = append(urls, s.robot.Sitemaps...)
	}
	return
}

func (s *sitemeta) SetInterval(d time.Duration) {
	s.Lock()
	s.interval = d
	s.Unlock()
}

func (s *sitemeta) addWaiting() time.Time {
	if s.waiting == 0 {
		s.nextTime = s.visited.lastTime.Add(s.interval)
	} else {
		s.nextTime = s.nextTime.Add(s.interval)
	}
	s.waiting++
	return s.nextTime
}

func (s *sitemeta) visitAt(at time.Time) {
	s.Lock()
	defer s.Unlock()
	s.visited.lastTime = at
	s.waiting--
}

func (s *sitemeta) updateRobots(code int, b []byte) error {
	var err error
	s.robot, err = robot.FromStatusAndBytes(code, b)
	return err
}

func siteRootURL(u *url.URL) *url.URL {
	uu := &url.URL{
		Scheme: u.Scheme,
		Host:   u.Host,
	}
	return uu
}

func siteRoot(u *url.URL) string {
	uu := url.URL{
		Scheme: u.Scheme,
		Host:   u.Host,
	}
	return uu.String()
}

type SitesMeta struct {
	crawler.NopController
	m map[string]*sitemeta
	sync.RWMutex
}

func NewSitesMeta() *SitesMeta {
	return &SitesMeta{
		m: make(map[string]*sitemeta),
	}
}

func (sp *SiteMeta) Exist(u *url.URL) bool {
	root := siteRoot(u)
	sp.RLock()
	defer sp.RUnlock()
	_, ok := sp.m[root]
	return ok
}

func (sp *SitesMeta) AddSite(u *url.URL) error {
	root := siteRootURL(u)
	sp.Lock()
	defer sp.Unlock()

	site, ok := sp.m[root.String()]
	if ok {
		return nil
	}

	var err error
	site, err = newSiteFromURL(root)
	if err != nil {
		return err
	}
	sp.m[root.String()] = site
	return nil
}

func urlToLink(urls []string) (links []*crawler.Link) {
	for _, s := range urls {
		u, err := url.Parse(s)
		if err != nil {
			logrus.Warnln(err)
			continue
		}
		links = append(links, &crawler.Link{
			URL: u,
		})
	}
	return
}

func (s *SitesMeta) Handle(resp *crawler.Response) (follow bool, links []*crawler.Link) {
	if resp.NewURL.Path != "/robots.txt" {
		if s.Exist(resp.NewURL) {
			return true, nil
		}
	}
	var err error
	root := siteRoot(resp.NewURL)
	// url := resp.NewURL.String()

	s.Lock()
	site, ok := s.m[root]
	if !ok {
		if site, err = newSiteFromURL(resp.NewURL); err != nil {
			return
		}
		s.m[root] = site
	}
	site.Lock()
	defer site.Unlock()
	s.Unlock()

	if err = site.updateRobots(resp.StatusCode, resp.Content); err != nil {
		logrus.Warnln(err)
	}
	return true, urlToLink(site.sitemapURLs())
}
