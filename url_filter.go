package crawler

import "net/url"

var (
	UrlBufSize = 64
)

func URLFilter(in <-chan *url.URL) <-chan *url.URL {
	out := make(chan *url.URL, UrlBufSize)
	go urlFilter(in, out)
	return out
}

func urlFilter(in <-chan *url.URL, out chan<- *url.URL) {
	for url := range in {
		// TODO: do real filter works
		out <- url
	}
	close(out)
}
