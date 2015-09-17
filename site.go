package crawler

import (
	"encoding/xml"
	"errors"
	"io/ioutil"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/fanyang01/crawler/sitemap"
	robot "github.com/temoto/robotstxt-go"
)

type URL struct {
	sitemap.URL
	Score   int64
	Visited struct {
		Count int
		Time  time.Time
	}
}

type URLMap struct {
	// NOTE: this mutex protects the map, NOT values stored in it.
	sync.RWMutex
	m map[string]*URL // using URI as key
}

type Site struct {
	Robot   *robot.RobotsData
	Client  *http.Client
	Root    string // http://example.com:8080, for robots.txt
	RootURL *url.URL
	Map     sitemap.Sitemap
	URLs    URLMap
}

var (
	ErrUnsupportedProtocol = errors.New("site: unsupported protocol")
	ErrNoHost              = errors.New("site: host can't be empty")
)

func NewSiteFromURL(u *url.URL) (*Site, error) {
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
	return &Site{
		Root:    uu.String(),
		RootURL: uu,
		URLs: URLMap{
			m: make(map[string]*URL),
		},
	}, nil
}

func NewSite(root string) (*Site, error) {
	u, err := url.Parse(root)
	if err != nil {
		return nil, err
	}
	return NewSiteFromURL(u)
}

func (m *URLMap) Add(u *URL) {
	uri := u.Loc.RequestURI()
	m.Lock()
	m.m[uri] = u
	m.Unlock()
}

func (m *URLMap) Get(URI string) (u *URL, ok bool) {
	m.RLock()
	u, ok = m.m[URI]
	m.RUnlock()
	return
}

func (site *Site) FetchRobots() error {
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
func GetBody(client *http.Client, url string) (body []byte, err error) {
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
func (site *Site) FetchSitemap() {
	f := func(absURL string) {
		// Although absURL may point to another site, we use settings of this site to get it
		body, err := GetBody(site.Client, absURL)
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
