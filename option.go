package crawler

import "net/http"

var DefaultClient = http.DefaultClient

type PoolOption struct {
	Size        int
	OutQueueLen int
}

type LinkParserOption struct {
	OutQueueLen int
}

type RespHandlerOption struct {
	OutQueueLen int
}

type URLFilterOption struct {
	OutQueueLen int
}

type RequestConstructorOption struct {
	OutQueueLen int
}

var DefaultOption = &Option{
	Pool: PoolOption{
		Size:        64,
		OutQueueLen: 64,
	},
	EnableUnkownLen: true,
	MaxHTMLLen:      1 << 20,
	LinkParser: LinkParserOption{
		OutQueueLen: 64,
	},
	RespHandler: RespHandlerOption{
		OutQueueLen: 64,
	},
	URLFilter: URLFilterOption{
		OutQueueLen: 64,
	},
	RequestConstructor: RequestConstructorOption{
		OutQueueLen: 64,
	},
}

type Option struct {
	Pool struct {
		Size        int
		OutQueueLen int
	}
	EnableUnkownLen     bool
	MaxHTMLLen          int64
	PriorityQueueBufLen int
	LinkParser          LinkParserOption
	RespHandler         RespHandlerOption
	URLFilter           URLFilterOption
	RequestConstructor  RequestConstructorOption
}
