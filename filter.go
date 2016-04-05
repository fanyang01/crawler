package crawler

import "net/url"

type filter struct {
	workerConn
	In     chan *Response
	Out    chan *Response
	NewOut chan *url.URL
	cw     *Crawler
}

func (cw *Crawler) newFilter() *filter {
	nworker := cw.opt.NWorker.Filter
	this := &filter{
		Out: make(chan *Response, nworker),
		cw:  cw,
	}
	cw.initWorker(this, nworker)
	return this
}

func (ft *filter) cleanup() {
	close(ft.Out)
}

func (ft *filter) work() {
	for resp := range ft.In {
		depth := ft.cw.store.GetDepth(resp.RequestURL)
		for _, link := range resp.links {
			link.Depth = depth + 1
			link.URL.Fragment = ""
			if ft.cw.ctrl.Accept(link) {
				// only handle new link
				if ft.cw.store.Exist(link.URL) {
					continue
				}
				if ft.cw.store.PutNX(&URL{
					Loc:   *link.URL,
					Depth: link.Depth,
				}) {
					link.follow = true
				}
			}
		}
		select {
		case ft.Out <- resp:
		case <-ft.quit:
			return
		}
	}
}
