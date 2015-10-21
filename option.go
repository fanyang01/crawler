package crawler

import "time"

var DefaultOption = &Option{
	MaxCacheSize:  1 << 25, // 32MB
	MinDelay:      10 * time.Second,
	RetryDuration: 30 * time.Second,
	NWorker: struct {
		Maker, Fetcher, Handler, Finder, Filter, Scheduler int
	}{
		Maker:     1,
		Fetcher:   4,
		Handler:   2,
		Finder:    4,
		Filter:    2,
		Scheduler: 2,
	},
}

type Option struct {
	MaxCacheSize    int64
	MinDelay        time.Duration
	RetryDuration   time.Duration
	RobotoAgent     string
	EnableUnkownLen bool
	MaxHTMLLen      int64
	NWorker         struct {
		Maker, Fetcher, Handler, Finder, Filter, Scheduler int
	}
}
