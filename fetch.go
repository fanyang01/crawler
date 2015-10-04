package crawler

import (
	"compress/flate"
	"compress/gzip"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	ErrTooManyEncodings    = errors.New("read response: too many encodings")
	ErrContentTooLong      = errors.New("read response: content length too long")
	ErrUnkownContentLength = errors.New("read response: unkown content length")
)

type Response struct {
	*http.Response
	requestURL      *url.URL
	Ready           bool     // body closed?
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
}

type fetcher struct {
	option *Option
	eQ     chan<- url.URL
	In     chan *Request
	Out    chan *Response
	Done   chan struct{}
}

func newFetcher(opt *Option, eQ chan<- url.URL) (fc *fetcher) {
	fc = &fetcher{
		option: opt,
		Out:    make(chan *Response, opt.Fetcher.QLen),
		eQ:     eQ,
	}
	return
}

func (fc *fetcher) Start() {
	n := fc.option.Fetcher.NWorker
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
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
			log.Printf("fetcher: %v\n", err)
			select {
			case fc.eQ <- *req.URL:
			case <-fc.Done:
				return
			}
			continue
		}
		select {
		case fc.Out <- resp:
		case <-fc.Done:
			return
		}
	}
}

type StdClient struct {
	client          *http.Client
	cache           *cachePool
	MaxHTMLLen      int64
	EnableUnkownLen bool
	pool            sync.Pool
}

func NewStdClient(opt *Option) *StdClient {
	client := &StdClient{
		client:          DefaultClient,
		MaxHTMLLen:      opt.MaxHTMLLen,
		EnableUnkownLen: opt.EnableUnkownLen,
		cache:           newCachePool(opt),
	}
	return client
}

func (ct *StdClient) Do(req *Request) (resp *Response, err error) {
	// First check cache
	var ok bool
	if resp, ok = ct.cache.Get(req.URL.String()); ok {
		return
	}

	resp = &Response{}
	resp.requestURL = req.URL
	resp.Response, err = ct.client.Do(req.Request)
	if err != nil {
		return
	}

	log.Printf("[%s] %s %s\n", resp.Status, req.Method, req.URL.String())

	// Only status code 2xx is ok
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err = errors.New(resp.Status)
		return
	}
	resp.parseHeader()
	// Only prefetch html content
	if CT_HTML.match(resp.ContentType) {
		if err = resp.ReadBody(
			ct.MaxHTMLLen,
			ct.EnableUnkownLen); err != nil {
			return
		}
		resp.CloseBody()
	}

	// Add to cache(NOTE: cache should use value rather than pointer)
	ct.cache.Add(resp)
	return
}

func (resp *Response) parseHeader() {
	var err error
	// Parse neccesary headers
	if t := resp.Header.Get("Date"); t != "" {
		resp.Date, err = time.Parse(http.TimeFormat, t)
		if err != nil {
			resp.Date = time.Now()
		}
	} else {
		resp.Date = time.Now()
	}
	if t := resp.Header.Get("Last-Modified"); t != "" {
		// on error, Time's zero value is used.
		resp.LastModified, _ = time.Parse(http.TimeFormat, t)
	}
	if t := resp.Header.Get("Expires"); t != "" {
		resp.Expires, err = time.Parse(http.TimeFormat, t)
		if err == nil {
			resp.Cacheable = true
		}
	}

	if a := resp.Header.Get("Age"); a != "" {
		if seconds, err := strconv.ParseInt(a, 0, 32); err == nil {
			resp.Age = time.Duration(seconds) * time.Second
		}
	}
	if c := resp.Header.Get("Cache-Control"); c != "" {
		if strings.HasPrefix(c, "s-maxage") {
			if seconds, err := strconv.ParseInt(
				strings.TrimPrefix(c, "s-maxage="), 0, 32); err == nil {
				resp.MaxAge = time.Duration(seconds) * time.Second
				resp.Cacheable = true
			}
		} else if strings.HasPrefix(c, "max-age") {
			if seconds, err := strconv.ParseInt(
				strings.TrimPrefix(c, "max-age="), 0, 32); err == nil {
				resp.MaxAge = time.Duration(seconds) * time.Second
				resp.Cacheable = true
			}
		}
		if resp.MaxAge != 0 {
			resp.Expires = resp.Date.Add(resp.MaxAge)
		}
	}
	baseurl := resp.Request.URL
	if baseurl == nil {
		baseurl = resp.requestURL
	}
	if l, err := resp.Location(); err == nil {
		baseurl, resp.Locations = l, l
	} else {
		resp.Locations = baseurl
	}
	if l, err := baseurl.Parse(resp.Header.Get("Content-Location")); err == nil {
		resp.ContentLocation = l
	}

	// Detect MIME types
	resp.detectMIME()
	return
}

func (resp *Response) ReadBody(maxLen int64, enableUnkownLen bool) error {
	if resp.Ready {
		return nil
	}
	defer resp.CloseBody()
	if resp.ContentLength > maxLen {
		return ErrContentTooLong
	}
	if resp.ContentLength < 0 && !enableUnkownLen {
		return ErrUnkownContentLength
	}

	// Uncompress http compression
	// We prefer Content-Encoding than Tranfer-Encoding
	var encoding string
	if encoding = resp.Header.Get("Content-Encoding"); encoding == "" {
		if len(resp.TransferEncoding) == 0 {
			encoding = "identity"
		} else if len(resp.TransferEncoding) == 1 {
			encoding = resp.TransferEncoding[0]
		} else {
			return fmt.Errorf("too many encodings: %v", resp.TransferEncoding)
		}
	}

	rc := resp.Body
	needclose := false // resp.Body.Close() is defered
	switch encoding {
	case "identity", "chunked":
	case "gzip":
		r, err := gzip.NewReader(rc)
		if err != nil {
			return err
		}
		rc, needclose = ioutil.NopCloser(r), true
	case "deflate":
		rc, needclose = flate.NewReader(rc), true
	default:
		return fmt.Errorf("unsupported content encoding: %s", encoding)
	}

	var err error
	resp.Content, err = ioutil.ReadAll(rc)
	if needclose {
		rc.Close()
	}
	return err
}

func (resp *Response) CloseBody() {
	if resp.Ready {
		return
	}
	resp.Body.Close()
	resp.Ready = true
}

func (resp *Response) detectMIME() {
	if t := resp.Header.Get("Content-Type"); t != "" {
		resp.ContentType = t
	} else if resp.Locations != nil || resp.ContentLocation != nil {
		var ext string
		if resp.Locations != nil {
			ext = path.Ext(resp.Locations.Path)
		}
		if ext == "" && resp.ContentLocation != nil {
			ext = path.Ext(resp.ContentLocation.Path)
		}
		if ext != "" {
			resp.ContentType = mime.TypeByExtension(ext)
		}
	}
}
