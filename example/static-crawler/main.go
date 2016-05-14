package main

import (
	"bufio"
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "net/http/pprof"

	"github.com/pkg/profile"
	"gopkg.in/inconshreveable/log15.v2"

	"github.com/fanyang01/crawler"
	"github.com/fanyang01/crawler/download"
	"github.com/fanyang01/crawler/extract"
	"github.com/fanyang01/crawler/queue/ratelimitq"
	"github.com/fanyang01/crawler/queue/ratelimitq/diskheap"
	"github.com/fanyang01/crawler/ratelimit"
	"github.com/fanyang01/crawler/sample/count"
	"github.com/fanyang01/crawler/sample/fingerprint"
	"github.com/fanyang01/crawler/sample/urltrie"
	"github.com/fanyang01/crawler/storage/boltstore"
)

var (
	dir           string
	seedfile      string
	offset, nseed int

	ctrl *Controller
)

func rate(host string) (time.Duration, int) {
	return time.Second, 10
}

func threshold(depth int) int {
	if depth <= 2 {
		depth = 0
	} else if depth > 6 {
		depth = 6
	}
	return 14 - 2*depth
}

func init() {
	flag.StringVar(&dir, "dir", "_testdata", "output directory")
	flag.StringVar(&seedfile, "f", "1000.csv", "file that provides seed URLs(one per line)")
	flag.IntVar(&offset, "offset", 1, "line offset in seeds file")
	flag.IntVar(&nseed, "n", 100, "number of lines starting from offset")

	flag.Parse()
}

func main() {
	defer profile.Start(profile.CPUProfile, profile.ProfilePath(".")).Stop()

	logger := log15.Root()
	logger.SetHandler(log15.MultiHandler(
		log15.LvlFilterHandler(
			log15.LvlInfo,
			log15.Must.FileHandler(filepath.Join(dir, "crawler.log"), log15.LogfmtFormat()),
		),
		log15.LvlFilterHandler(log15.LvlError, log15.StdoutHandler),
	))

	csv, err := os.Open(seedfile)
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

	pattern := &extract.Pattern{
		File: []string{
			"", "*.?htm?", `/[^\.]*/`,
			`/.*\.(jpg|JPG|png|PNG|jpeg|JPEG|gif|GIF)/`,
			`/.*\.(php|jsp|aspx|asp|cgi|do)/`,
			"*.css", "*.js",
		},
		// ExcludeFile: []string{
		// 	"*.doc?", "*.xls?", "*.ppt?",
		// 	"*.pdf", "*.rar", "*.zip",
		// 	"*.ico", "*.apk", "*.exe",
		// 	"*.mp4", "*.mkv",
		// },
	}
	ctrl = &Controller{
		extractor: &extract.Extractor{
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
			SpanHosts:  true,
			SubDomain:  true,
			ResolveIP:  true,
		},
		downloader: &download.Downloader{
			Dir: dir,
		},
		trie:        urltrie.NewHosts(threshold),
		count:       count.NewHosts(),
		fingerprint: fingerprint.NewStore(0, 4, 4096),
		limiter:     ratelimit.New(rate),
		logger:      logger.New("worker", "controller"),
	}
	ctrl.complete.hosts = make(map[string]bool)

	store, err := boltstore.New(filepath.Join(dir, "bolt.db"), nil, nil)
	if err != nil {
		log.Fatal(err)
	}
	// queue, err := diskqueue.NewDefault(store.DB)
	queue := ratelimitq.NewWaitQueue(&ratelimitq.Option{
		Limit:     ctrl.Interval,
		Secondary: diskheap.New(store.DB, []byte("HEAP"), 16),
	})

	go func() {
		http.Handle("/count/", http.HandlerFunc(handleCount))
		log.Fatal(http.ListenAndServe("localhost:7869", nil))
	}()

	cw := crawler.New(&crawler.Config{
		Controller: ctrl,
		Logger:     logger,
		Store:      store,
		Queue:      queue,
	})
	if err := cw.Crawl(urls[offset-1 : offset-1+nseed]...); err != nil {
		log.Fatal(err)
	}
	cw.Wait()
}
