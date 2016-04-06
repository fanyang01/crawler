package crawler

import (
	"time"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/simplifiedchinese"
)

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
		Maker, Fetcher, Handler, Finder, Filter, Scheduler int
	}
	Electron struct {
		ExecPath string
		AppDir   string
	}
	UnknownEncoding     encoding.Encoding
	UnknownEncodingName string
}

var (
	DefaultOption = &Option{
		UserAgent:     browserAgant,
		RobotAgent:    "gocrawler",
		MaxCacheSize:  1 << 25, // 32MB
		MaxHTML:       1 << 20, // iMB
		MinDelay:      10 * time.Second,
		RetryDuration: 30 * time.Second,
		MaxRetry:      4,
		NWorker: struct {
			Maker, Fetcher, Handler, Finder, Filter, Scheduler int
		}{
			Maker:     8,
			Fetcher:   32,
			Handler:   8,
			Finder:    32,
			Filter:    8,
			Scheduler: 8,
		},
		UnknownEncoding:     simplifiedchinese.GBK,
		UnknownEncodingName: "gbk",
	}
)
