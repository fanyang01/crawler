package crawler

import "net/url"

var (
	UrlBufSize = 64
)

type URLFilter interface {
	Filter(*url.URL) bool
}

func FilterURL(in <-chan *url.URL, filter URLFilter) <-chan *url.URL {
	out := make(chan *url.URL, UrlBufSize)
	go func() {
		for u := range in {
			if ok := filter.Filter(u); ok {
				out <- u
			}
		}
		close(out)
	}()
	return out
}

func newRequest(u *url.URL) *Request {
	return &Request{
		method: "GET",
		url:    u.String(),
	}
}

func NewRequest(in <-chan *URL) <-chan *Request {
	out := make(chan *Request, UrlBufSize)
	go func() {
		for u := range in {
			out <- newRequest(u.Loc)
		}
	}()
	return out
}
