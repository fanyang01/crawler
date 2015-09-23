package crawler

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"sync"

	"github.com/fanyang01/crawler/sitemap"
	robot "github.com/temoto/robotstxt-go"
)

type URLMap struct {
	// NOTE: this mutex protects the map, NOT values stored in it.
	sync.RWMutex
	m map[string]URL // using URI as key
}

type Site struct {
	Robot   *robot.RobotsData
	Client  *http.Client
	Root    string // http://example.com:8080, for robots.txt
	RootURL *url.URL
	Map     sitemap.Sitemap
}

var (
	ErrUnsupportedProtocol = errors.New("site: unsupported protocol")
	ErrNoHost              = errors.New("site: host can't be empty")
)

func newSiteFromURL(u *url.URL) (*Site, error) {
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
	site := &Site{
		Root:    uu.String(),
		RootURL: uu,
	}
	return site, nil
}

func newSite(root string) (*Site, error) {
	u, err := url.Parse(root)
	if err != nil {
		return nil, err
	}
	return newSiteFromURL(u)
}

func (site *Site) fetchRobots() error {
	client := site.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Get(site.Root + "/robot.txt")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	site.Robot, err = robot.FromStatusAndBytes(resp.StatusCode, body)
	return err
}

// Do http GET request and read response body. Only status code 2xx is ok.
func getBody(client *http.Client, url string) (body []byte, err error) {
	var resp *http.Response
	if client == nil {
		client = http.DefaultClient
	}
	resp, err = client.Get(url)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 && resp.StatusCode >= 300 {
		return nil, errors.New(resp.Status)
	}
	body, err = ioutil.ReadAll(resp.Body)
	return
}

// All errors are ignored.
// TODO: log error as warning
func (site *Site) fetchSitemap() {
	f := func(absURL string) {
		// Although absURL may point to another site, we use settings of this site to get it
		body, err := getBody(site.Client, absURL)
		if err != nil {
			return
		}
		var smap sitemap.Sitemap
		if err := xml.Unmarshal(body, &smap); err != nil {
			return
		}
		site.Map.URLSet = append(site.Map.URLSet, smap.URLSet...)
	}

	// Try to get sitemap.xml at root directory
	rootSitemap := site.Root + "/sitemap.xml"
	f(rootSitemap)

	for _, absURL := range site.Robot.Sitemaps {
		if absURL == rootSitemap {
			continue
		}
		f(absURL)
	}
}

func siteRoot(u url.URL) string {
	uu := url.URL{
		Scheme: u.Scheme,
		Host:   u.Host,
	}
	return uu.String()
}

func (cw *Crawler) addSite(u url.URL) error {
	root := siteRoot(u)
	cw.sites.Lock()
	defer cw.sites.Unlock()
	site, ok := cw.sites.m[root]
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
		cw.pQueue.Push(&URL{URL: u})
	}
	cw.sites.m[root] = site
	return nil
}

func (cw *Crawler) testRobot(u url.URL) bool {
	cw.sites.RLock()
	defer cw.sites.RUnlock()
	site, ok := cw.sites.m[siteRoot(u)]
	if !ok || site.Robot == nil {
		return false
	}
	return site.Robot.TestAgent(u.Path, cw.option.RobotoAgent)
}
