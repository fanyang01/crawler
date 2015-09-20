package crawler

import (
	"bytes"
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
	"time"
)

type fetcher struct {
	cache   *cachePool
	client  *http.Client
	option  *Option
	workers chan struct{}
	In      chan *Request
	Out     chan *Response
}

func newFetcher(opt *Option) (fc *fetcher) {
	fc = &fetcher{
		option:  opt,
		Out:     make(chan *Response, opt.Fetcher.OutQueueLen),
		cache:   newCachePool(),
		workers: make(chan struct{}, opt.Fetcher.NumOfWorkers),
	}
	return
}

func (fc *fetcher) Start() {
	for i := 0; i < fc.option.Fetcher.NumOfWorkers; i++ {
		fc.workers <- struct{}{}
	}
	go func() {
		for req := range fc.In {
			<-fc.workers
			go func(r *Request) {
				fc.do(r)
				fc.workers <- struct{}{}
			}(req)
		}
		close(fc.Out)
	}()
}

func (fc *fetcher) do(req *Request) {
	// First check cache
	if resp, ok := fc.cache.Get(req.url); ok {
		fc.Out <- resp
		return
	}
	resp, err := fc.fetch(req)
	if err != nil {
		log.Printf("fetcher: %v\n", err)
		return
	}
	// Add to cache
	fc.cache.Add(resp)
	fc.Out <- resp
}

var (
	ErrTooManyEncodings    = errors.New("read response: too many encodings")
	ErrContentTooLong      = errors.New("read response: content length too long")
	ErrUnkownContentLength = errors.New("read response: unkown content length")
)

func (fc *fetcher) fetch(r *Request) (resp *Response, err error) {
	if r.method == "" {
		r.method = "GET"
	}
	if r.client == nil {
		r.client = DefaultClient
	}

	resp = new(Response)
	resp.requestURL, err = url.Parse(r.url)
	if err != nil {
		return
	}

	var req *http.Request
	req, err = http.NewRequest(r.method, r.url, bytes.NewReader(r.body))
	if err != nil {
		return
	}
	req.Header.Set("User-Agent", fc.option.RobotoAgent)
	if r.config != nil {
		r.config(req)
	}

	resp.Response, err = r.client.Do(req)
	if err != nil {
		return
	}

	log.Printf("[%s] %s %s\n", resp.Status, r.method, r.url)

	// Only status code 2xx is ok
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err = errors.New(resp.Status)
		return
	}
	resp.parseHeader()
	// Only prefetch html content
	if CT_HTML.match(resp.ContentType) {
		if err = resp.ReadBody(
			fc.option.MaxHTMLLen,
			fc.option.EnableUnkownLen); err != nil {
			return
		}
		resp.CloseBody()
	}
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
	resp.Ready = true
	return err
}

func (resp *Response) CloseBody() {
	if resp.Ready {
		return
	}
	resp.Body.Close()
	resp.Ready = true
}

// When nessary, detectMIME will prefetch 512 bytes from body
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
