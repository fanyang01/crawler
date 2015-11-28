package crawler

import "net/url"

type filter struct {
	workerConn
	In     chan *Link
	Out    chan *Link
	NewOut chan *url.URL
	ctl    Controller
	store  URLStore
}

func (cw *Crawler) newFilter() *filter {
	nworker := cw.opt.NWorker.Filter
	this := &filter{
		Out:   make(chan *Link, nworker),
		ctl:   cw.ctl,
		store: cw.urlStore,
	}
	this.nworker = nworker
	this.wg = &cw.wg
	this.quit = cw.quit
	return this
}

func (ft *filter) cleanup() {
	close(ft.Out)
}

func (ft *filter) work() {
	for link := range ft.In {
		depth := ft.store.GetDepth(link.Base)
		for _, anchor := range link.Anchors {
			anchor.Depth = depth + 1
			anchor.URL.Fragment = ""
			if ft.ctl.Accept(anchor) {
				// only handle new link
				if ft.store.Exist(anchor.URL) {
					continue
				}
				if ft.store.PutNX(&URL{
					Loc:   *anchor.URL,
					Depth: anchor.Depth,
				}) {
					anchor.ok = true
				}
			}
		}
		select {
		case ft.Out <- link:
		case <-ft.quit:
			return
		}
	}
}
