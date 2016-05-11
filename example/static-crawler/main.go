package main

import (
	"bufio"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	_ "net/http/pprof"

	"github.com/pkg/profile"
	"gopkg.in/inconshreveable/log15.v2"

	"github.com/fanyang01/crawler"
	"github.com/fanyang01/crawler/download"
	"github.com/fanyang01/crawler/extract"
	"github.com/fanyang01/crawler/media"
	"github.com/fanyang01/crawler/queue/diskqueue"
	"github.com/fanyang01/crawler/ratelimit"
	"github.com/fanyang01/crawler/sim/urltrie"
	"github.com/fanyang01/crawler/storage/boltstore"
)

type Controller struct {
	crawler.NopController
	Extractor  *extract.Extractor
	Downloader *download.SimDownloader
	Trie       *urltrie.MultiHost
	Limiter    *ratelimit.Limit
	logger     log15.Logger
}

var freelist = download.NewFreeList(1<<20, 32)

func (c *Controller) Handle(r *crawler.Response, ch chan<- *url.URL) {
	buf := freelist.Get()
	defer freelist.Put(buf)

	tee := io.TeeReader(r.Body, buf)
	isHTML := media.IsHTML(r.ContentType)
	similar, err := c.Downloader.Handle(r.URL, tee, isHTML)
	if err != nil {
		c.logger.Error("download error", "err", err, "url", r.URL)
		return
	}
	depth, _ := r.Context().Depth()
	c.logger.Info("",
		"url", r.URL, "depth", depth,
		"isHTML", isHTML, "similar", similar,
	)
	if isHTML && !similar {
		if err := c.Extractor.Extract(r, buf, ch); err != nil {
			c.logger.Error("extract link", "err", err)
		}
	}
}

func (c *Controller) Accept(r *crawler.Response, u *url.URL) bool {
	return r.URL.Host == u.Host && c.Trie.Add(u)
}

func (c *Controller) Sched(r *crawler.Response, u *url.URL) crawler.Ticket {
	d := c.Limiter.Reserve(u)
	return crawler.Ticket{At: time.Now().Add(d)}
}

func rate(host string) (time.Duration, int) {
	return time.Second, 2
}

func threshold(depth int) int {
	if depth <= 2 {
		depth = 0
	} else if depth > 8 {
		depth = 8
	}
	return 20 - 2*depth
}

func main() {
	defer profile.Start(profile.MemProfile, profile.ProfilePath(".")).Stop()

	logger := log15.Root()
	logger.SetHandler(log15.MultiHandler(
		log15.Must.FileHandler("_testdata/crawler.log", log15.LogfmtFormat()),
		log15.StdoutHandler,
	))

	pattern := &extract.Pattern{
		File: []string{
			"", "*.?htm?", `/[^\.]*/`,
			`/.*\.(jpg|JPG|png|PNG|jpeg|JPEG|gif|GIF)/`,
			`/.*\.(php|jsp|aspx|asp|cgi|do)/`,
			"*.css", "*.js",
			"*http?://*",
		},
		// ExcludeFile: []string{
		// 	"*.doc?", "*.xls?", "*.ppt?",
		// 	"*.pdf", "*.rar", "*.zip",
		// 	"*.ico", "*.apk", "*.exe",
		// 	"*.mp4", "*.mkv",
		// },
	}
	ctrl := &Controller{
		Extractor: &extract.Extractor{
			Matcher:  extract.MustCompile(pattern),
			MaxDepth: 4,
			Pos: []struct{ Tag, Attr string }{
				{"a", "href"},
				{"img", "src"},
				{"link", "href"},
				{"script", "src"},
			},
			SniffFlags: extract.SniffWindowLocation,
			Redirect:   true,
		},
		Downloader: &download.SimDownloader{
			Dir:      "_testdata",
			Distance: 0,
			Shingle:  4,
			MaxToken: 4096,
		},
		Trie:    urltrie.NewMultiHost(threshold),
		Limiter: ratelimit.New(rate),
		logger:  logger.New("worker", "controller"),
	}

	store, err := boltstore.New("_testdata/bolt.db", nil, nil)
	if err != nil {
		log.Fatal(err)
	}
	queue, err := diskqueue.NewDefault("_testdata/queue.db")
	if err != nil {
		log.Fatal(err)
	}

	csv, err := os.Open("1000.csv")
	if err != nil {
		log.Fatal(err)
	}
	var urls []string
	scanner := bufio.NewScanner(csv)
	for scanner.Scan() {
		url := scanner.Text()
		if !strings.HasPrefix(url, "#") {
			urls = append(urls, url)
		}
	}
	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}
	csv.Close()

	go func() {
		log.Println(http.ListenAndServe("localhost:7869", nil))
	}()

	cw := crawler.NewCrawler(&crawler.Config{
		Controller: ctrl,
		Logger:     logger,
		Store:      store,
		Queue:      queue,
	})
	if err := cw.Crawl(urls[100:200]...); err != nil {
		log.Fatal(err)
	}
	cw.Wait()
}
