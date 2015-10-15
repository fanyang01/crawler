package crawler

import "time"

var DefaultOption = &Option{
	MaxCacheSize: 1 << 25, // 32MB
	MinDelay:     10 * time.Second,
	RetryDelay:   10 * time.Second,
	RobotoAgent:  "I'm a Roboto",
	NWorker: struct {
		Maker, Fetcher, Handler, Parser, Filter, Scheduler int
	}{
		Maker:     1,
		Fetcher:   32,
		Handler:   16,
		Parser:    32,
		Filter:    2,
		Scheduler: 2,
	},
}

type Option struct {
	MaxCacheSize    int64
	MinDelay        time.Duration
	RetryDelay      time.Duration
	RobotoAgent     string
	EnableUnkownLen bool
	MaxHTMLLen      int64
	NWorker         struct {
		Maker, Fetcher, Handler, Parser, Filter, Scheduler int
	}
}
