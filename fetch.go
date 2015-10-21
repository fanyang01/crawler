package crawler

import (
	"errors"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/PuerkitoBio/goquery"
)

var (
	ErrTooManyEncodings    = errors.New("read response: too many encodings")
	ErrContentTooLong      = errors.New("read response: content length too long")
	ErrUnkownContentLength = errors.New("read response: unkown content length")
)

// Response contains a http response some metadata.
// Note the body of response may be read or not, depending on
// the type of content and the size of content. Call ReadBody to
// safely read and close the body. Optionally, you can access Body
// directly but do NOT close it.
type Response struct {
	*http.Response
	// RequestURL is the original url used to do request that finally
	// produces this response.
	RequestURL      *url.URL
	ready           bool     // body read and closed?
	Locations       *url.URL // distinguish with method Location
	ContentLocation *url.URL
	ContentType     string
	Content         []byte
	Date            time.Time
	LastModified    time.Time
	Expires         time.Time
	Cacheable       bool
	Age             time.Duration
	MaxAge          time.Duration
	// content will be parsed into document only if neccessary.
	document *goquery.Document
}

type fetcher struct {
	workerConn
	In     <-chan *Request
	Out    chan *Response
	ErrOut chan *url.URL
	cache  *cachePool
	store  URLStore
}

func (cw *Crawler) newFetcher() *fetcher {
	nworker := cw.opt.NWorker.Fetcher
	this := &fetcher{
		Out:    make(chan *Response, nworker),
		ErrOut: make(chan *url.URL, nworker),
		store:  cw.urlStore,
		cache:  newCachePool(cw.opt.MaxCacheSize),
	}
	this.nworker = nworker
	this.WG = &cw.wg
	this.Done = cw.done
	return this
}

func (fc *fetcher) cleanup() { close(fc.Out) }

func (fc *fetcher) work() {
	for req := range fc.In {
		// First check cache
		var resp *Response
		var ok bool
		if resp, ok = fc.cache.Get(req.URL.String()); !ok {
			var err error
			resp, err = req.Client.Do(req)
			if err != nil {
				log.Printf("fetcher: %v\n", err)
				select {
				case fc.ErrOut <- req.URL:
				case <-fc.Done:
					return
				}
				continue
			}
			// Add to cache
			fc.cache.Add(resp)
		}
		fc.store.UpdateVisit(req.URL, resp.Date, resp.LastModified)
		// redirect
		if resp.Locations.String() != req.URL.String() {
			fc.store.UpdateVisit(resp.Locations, resp.Date, resp.LastModified)
		}
		select {
		case fc.Out <- resp:
		case <-fc.Done:
			return
		}
	}
}
