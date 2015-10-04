package crawler

import (
	"net/http"
	"time"
)

var DefaultClient = http.DefaultClient

type WorkerOption struct {
	NWorker int
	QLen    int
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
		NWorker: 32,
		QLen:    64,
	},
	EnableUnkownLen: true,
	MaxHTMLLen:      1 << 20,
	LinkParser: WorkerOption{
		QLen:    32,
		NWorker: 64,
	},
	URLFilter: WorkerOption{
		NWorker: 16,
		QLen:    64,
	},
	RequestMaker: WorkerOption{
		NWorker: 1,
		QLen:    64,
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
}
