package crawler

import "time"

const (
	browserAgant = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/46.0.2490.71 Safari/537.36"
)

type Option struct {
	UserAgent      string
	RobotAgent     string
	MinDelay       time.Duration
	RobotoAgent    string
	FollowRedirect bool
	NWorker        struct {
		Maker, Fetcher, Handler, Scheduler int
	}
}

var (
	DefaultOption = &Option{
		UserAgent:  browserAgant,
		RobotAgent: "gocrawler",
		MinDelay:   10 * time.Second,
		NWorker: struct {
			Maker, Fetcher, Handler, Scheduler int
		}{
			Maker:     8,
			Fetcher:   32,
			Handler:   32,
			Scheduler: 8,
		},
	}
)
