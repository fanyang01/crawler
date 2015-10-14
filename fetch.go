package crawler

import (
	"errors"
	"net/http"
	"net/url"
	"sync"
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
	errQ    chan<- *url.URL
	store   URLStore
	In      <-chan *Request
	Out     chan *Response
	Done    chan struct{}
	nworker int
}

func newFetcher(nworker int, in <-chan *Request, done chan struct{},
	errQ chan<- *url.URL, store URLStore) *fetcher {
	return &fetcher{
		Out:     make(chan *Response, nworker),
		In:      in,
		Done:    done,
		nworker: nworker,
		store:   store,
		errQ:    errQ,
	}
}

func (fc *fetcher) start() {
	var wg sync.WaitGroup
	wg.Add(fc.nworker)
	for i := 0; i < fc.nworker; i++ {
		go func() {
			fc.work()
			wg.Done()
		}()
	}
	go func() {
		wg.Wait()
		close(fc.Out)
	}()
}

func (fc *fetcher) work() {
	for req := range fc.In {
		resp, err := req.Client.Do(req)
		if err != nil {
			// log.Printf("fetcher: %v\n", err)
			h := fc.store.WatchP(URL{Loc: *req.URL})
			u := h.V()
			u.Status = U_Error
			h.Unlock()
			select {
			case fc.errQ <- req.URL:
			case <-fc.Done:
				return
			}
			continue
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
