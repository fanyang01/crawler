package crawler

import "net/http"

var DefaultClient = http.DefaultClient

type WorkerOption struct {
	NumOfWorkers int
	OutQueueLen  int
}

var DefaultOption = &Option{
	RobotoAgent:         "I'm a Roboto",
	PriorityQueueBufLen: 2,
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
	RespHandler: WorkerOption{
		OutQueueLen:  64,
		NumOfWorkers: 4,
	},
	URLFilter: WorkerOption{
		NumOfWorkers: 16,
		OutQueueLen:  64,
	},
	RequestConstructor: WorkerOption{
		OutQueueLen: 64,
	},
}

type Option struct {
	RobotoAgent         string
	EnableUnkownLen     bool
	MaxHTMLLen          int64
	PriorityQueueBufLen int
	Fetcher             WorkerOption
	LinkParser          WorkerOption
	RespHandler         WorkerOption
	URLFilter           WorkerOption
	RequestConstructor  WorkerOption
}
