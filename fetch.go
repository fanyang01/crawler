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
	conn
	In     <-chan *Request
	Out    chan *Response
	ErrOut chan<- *url.URL
	cache  *cachePool
	store  URLStore
}

func newFetcher(nworker int, store URLStore, maxCacheSize int64) *fetcher {
	return &fetcher{
		Out:   make(chan *Response, nworker),
		store: store,
		cache: newCachePool(maxCacheSize),
	}
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
				h := fc.store.WatchP(URL{Loc: *req.URL})
				u := h.V()
				u.Status = U_Error
				h.Unlock()
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
		h := fc.store.WatchP(URL{Loc: *resp.Locations})
		u := h.V()
		u.Visited.Count++
		u.Visited.Time = resp.Date
		u.LastModified = resp.LastModified
		u.Status = U_Fetched
		h.Unlock()
		// redirect
		if resp.Locations.String() != req.URL.String() {
			if h := fc.store.Watch(*req.URL); h != nil {
				u := h.V()
				u.Visited.Count++
				u.Visited.Time = resp.Date
				u.LastModified = resp.LastModified
				u.Status = U_Redirected
				h.Unlock()
			}
		}
		select {
		case fc.Out <- resp:
		case <-fc.Done:
			return
		}
	}
}
