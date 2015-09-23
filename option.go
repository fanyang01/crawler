package crawler

import (
	"net/http"
	"time"
)

var DefaultClient = http.DefaultClient

type WorkerOption struct {
	NumOfWorkers int
	OutQueueLen  int
}

type QueueOption struct {
	BufLen, MaxLen int
}

var DefaultOption = &Option{
	MaxCacheSize:  1 << 25, // 32MB
	DefaultClient: http.DefaultClient,
	ErrorQueueLen: 128,
	MinDelay:      10 * time.Second,
	RetryDelay:    10 * time.Second,
	RobotoAgent:   "I'm a Roboto",
	PriorityQueue: QueueOption{
		BufLen: 32,
		MaxLen: 2048,
	},
	TimeQueue: QueueOption{
		BufLen: 32,
		MaxLen: 2048,
	},
	Fetcher: WorkerOption{
		NumOfWorkers: 64,
		OutQueueLen:  64,
	},
	EnableUnkownLen: true,
	MaxHTMLLen:      1 << 20,
	LinkParser: WorkerOption{
		OutQueueLen:  64,
		NumOfWorkers: 64,
	},
	URLFilter: WorkerOption{
		NumOfWorkers: 32,
		OutQueueLen:  64,
	},
	RequestMaker: WorkerOption{
		OutQueueLen: 64,
	},
	SiteExplorer: WorkerOption{
		OutQueueLen: 64,
	},
}

type Option struct {
	MaxCacheSize    int
	DefaultClient   *http.Client
	ErrorQueueLen   int
	MinDelay        time.Duration
	RetryDelay      time.Duration
	RobotoAgent     string
	EnableUnkownLen bool
	MaxHTMLLen      int64
	PriorityQueue   QueueOption
	TimeQueue       QueueOption
	Fetcher         WorkerOption
	LinkParser      WorkerOption
	URLFilter       WorkerOption
	RequestMaker    WorkerOption
	SiteExplorer    WorkerOption
}
