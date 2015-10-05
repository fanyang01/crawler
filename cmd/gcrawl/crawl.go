package main

import (
	"log"
	"time"

	"github.com/fanyang01/crawler"
	"github.com/fanyang01/crawler/cmd/gcrawl/task"
	"github.com/ryanuber/go-glob"

	flag "github.com/ogier/pflag"
)

var (
	taskFile = flag.StringP("task", "t", "task.toml", "Task file in TOML format")
)

type ctrl struct {
	crawler.Ctrl
	tsk *task.Task
}

func (c *ctrl) Schedule(u crawler.URL) (score int64, at time.Time) {
	str := u.Loc.String()
	for _, ft := range c.tsk.Filter {
		if glob.Glob(ft.Pattern, str) {
			score = ft.Score / 2
			break
		}
	}
	isTarget := false
	var pscore int64
	for _, tgt := range c.tsk.Target {
		if glob.Glob(tgt.Pattern, str) {
			pscore = int64(tgt.Priority * 512.0)
			if u.Visited.Count == 0 {
				at = time.Now()
			} else {
				at = u.Visited.Time.Add(tgt.Frequence)
			}
			isTarget = true
			break
		}
	}
	if isTarget {
		score += pscore
	} else if u.Visited.Count > 0 {
		score = 0
	}
	return
}

func (c *ctrl) Handle(resp *crawler.Response, doc *crawler.Doc) {
	str := resp.Locations.String()
	for _, tgt := range c.tsk.Target {
		if glob.Glob(tgt.Pattern, str) {
			log.Printf("[ OK ] %s", str)
			break
		}
	}
}

func main() {
	flag.Parse()
	tsk, err := task.ReadTask(*taskFile)
	if err != nil {
		log.Fatalf("read %s: %v", *taskFile, err)
	}
	ct := &ctrl{
		tsk: tsk,
	}
	cw := crawler.NewCrawler(ct, nil)
	for _, seed := range tsk.Seed {
		cw.AddSeeds(seed.URL)
	}
	cw.Crawl()
	time.Sleep(40 * 1E9)
	cw.Stop()
	time.Sleep(1 * 1E9)
}
