package crawler

import "time"

const (
	browserAgant = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/46.0.2490.71 Safari/537.36"
)

type Option struct {
	UserAgent     string
	RobotAgent    string
	MaxCacheSize  int64
	MinDelay      time.Duration
	RetryDuration time.Duration
	MaxRetry      int
	RobotoAgent   string
	MaxHTML       int64
	NWorker       struct {
		Maker, Fetcher, Handler, Scheduler int
	}
	Electron struct {
		ExecPath string
		AppDir   string
	}
}

var (
	DefaultOption = &Option{
		UserAgent:     browserAgant,
		RobotAgent:    "gocrawler",
		MaxCacheSize:  1024,
		MaxHTML:       1 << 20, // iMB
		MinDelay:      10 * time.Second,
		RetryDuration: 20 * time.Second,
		MaxRetry:      4,
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
