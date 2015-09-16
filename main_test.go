package crawler

import (
	"log"
	"testing"
	"time"
)

func TestCrawler(t *testing.T) {
	log.SetFlags(0)
	seeds := []string{
		// "https://fanyang01.github.io",
		// "https://news.ycombinator.com/",
		// "http://weibo.cn/",
		"http://localhost:6060",
	}
	cw := NewCrawler(nil, nil)
	if err := cw.Begin(seeds...); err != nil {
		t.Fatal(err)
	}
	cw.Crawl()
	time.Sleep(30 * 1E9)
}
