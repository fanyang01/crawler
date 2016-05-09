package main

import (
	"bufio"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"gopkg.in/inconshreveable/log15.v2"

	_ "net/http/pprof"

	"github.com/fanyang01/crawler"
	"github.com/fanyang01/crawler/download"
	"github.com/fanyang01/crawler/extract"
	"github.com/fanyang01/crawler/media"
	"github.com/fanyang01/crawler/queue/diskqueue"
	"github.com/fanyang01/crawler/ratelimit"
	"github.com/fanyang01/crawler/sim/urltrie"
	"github.com/fanyang01/crawler/storage/boltstore"
	"github.com/fanyang01/crawler/util"
)

type Controller struct {
	crawler.NopController
	Extractor  *extract.Extractor
	Downloader *download.SimDownloader
	Trie       *urltrie.MultiHost
	Limiter    *ratelimit.Limit
	logger     log15.Logger
}

func (c *Controller) Handle(r *crawler.Response, ch chan<- *url.URL) {
	readers, readAll := util.DumpReader(io.LimitReader(r.Body, 1<<19), 2)
	done := make(chan struct{})
	go func() {
		if err := c.Extractor.Extract(r, readers[0], ch); err != nil {
			c.logger.Error("extract link", "err", err)
		}
		io.Copy(ioutil.Discard, readers[0])
		close(done)
	}()

	is := media.IsHTML(r.ContentType)
	sim, err := c.Downloader.Handle(r.URL, readers[1], is)
	if err != nil {
		c.logger.Error("download", "err", err, "url", r.URL)
	} else if sim {
		c.logger.Info("found similar page, download canceled", "url", r.URL)
	} else {
		c.logger.Info("download finished", "url", r.URL)
	}
	io.Copy(ioutil.Discard, readers[1])
	<-readAll
	<-done
}

func (c *Controller) Accept(r *crawler.Response, u *url.URL) bool {
	return c.Trie.Add(u)
}

func (c *Controller) Sched(r *crawler.Response, u *url.URL) crawler.Ticket {
	d := c.Limiter.Reserve(u)
	return crawler.Ticket{At: time.Now().Add(d)}
}

func rate(host string) (time.Duration, int) {
	return time.Second, 2
}

func limit(depth int) int {
	return 64 - 2*depth
}

func main() {
	logger := log15.Root()
	logger.SetHandler(log15.MultiHandler(
		log15.Must.FileHandler("_testdata/crawler.log", log15.LogfmtFormat()),
		log15.StdoutHandler,
	))

	pattern := &extract.Pattern{
		ExcludeFile: []string{
			"*.doc?", "*.xls?", "*.ppt?",
			"*.pdf", "*.rar", "*.zip",
			"*.ico",
		},
	}
	ctrl := &Controller{
		Extractor: &extract.Extractor{
			Matcher:  extract.MustCompile(pattern),
			MaxDepth: 4,
			Destination: []struct{ Tag, Attr string }{
				{"a", "href"},
				{"img", "src"},
				{"link", "href"},
				{"script", "src"},
			},
			SniffFlags: extract.SniffWindowLocation,
		},
		Downloader: &download.SimDownloader{
			Dir:      "_testdata",
			Distance: 0,
			Shingle:  4,
			MaxToken: 4096,
		},
		Trie:    urltrie.NewMultiHost(limit),
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
	if err := cw.Crawl(urls[:100]...); err != nil {
		// if err := cw.Crawl("http://www.homekoo.com"); err != nil {
		log.Fatal(err)
	}
	cw.Wait()
}
