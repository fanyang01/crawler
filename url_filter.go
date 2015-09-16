package crawler

var (
	UrlBufSize = 64
)

type filter struct {
	In     chan *URL
	Out    chan *URL
	option *Option
}

type URLFilter interface {
	Filter(*URL) bool
}

func newFilter(opt *Option) *filter {
	return &filter{
		Out:    make(chan *URL, opt.URLFilter.OutQueueLen),
		option: opt,
	}
}

func (ft *filter) Start(filters ...URLFilter) {
	go func() {
		for u := range ft.In {
			var ok = true
			for _, filter := range filters {
				if ok = filter.Filter(u); !ok {
					break
				}
			}
			if ok {
				ft.Out <- u
			}
		}
		close(ft.Out)
	}()
}
