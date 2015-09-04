package crawler

var (
	UrlBufSize = 64
)

func URLFilter(in <-chan *URL) <-chan *URL {
	out := make(chan *URL, UrlBufSize)
	go urlFilter(in, out)
	return out
}

func urlFilter(in <-chan *URL, out chan<- *URL) {
	for url := range in {
		// TODO: do real filter works
		out <- url
	}
	close(out)
}
