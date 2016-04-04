package crawler

import (
	"compress/flate"
	"compress/gzip"
	"errors"
	"fmt"
	"io/ioutil"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
)

var (
	ErrTooManyEncodings    = errors.New("read response: too many encodings")
	ErrContentTooLong      = errors.New("read response: content length too long")
	ErrUnkownContentLength = errors.New("read response: unkown content length")
)

type fetcher struct {
	workerConn
	In     <-chan *Request
	Out    chan *Response
	ErrOut chan *url.URL
	cache  *cachePool
	cw     *Crawler
}

func (cw *Crawler) newFetcher() *fetcher {
	nworker := cw.opt.NWorker.Fetcher
	this := &fetcher{
		Out:   make(chan *Response, nworker),
		cache: newCachePool(cw.opt.MaxCacheSize),
		cw:    cw,
	}
	cw.initWorker(this, nworker)
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
			// Prefetch html document
			if err == nil && CT_HTML.match(resp.ContentType) {
				err = resp.ReadBody(fc.cw.opt.MaxHTML)
			}
			if err != nil {
				logrus.Errorf("fetcher: %v", err)
				select {
				case fc.ErrOut <- req.URL:
				case <-fc.quit:
					return
				}
				continue
			}
			// Add to cache
			fc.cache.Add(resp)
		}
		// Redirected response is threated as the response of original URL
		fc.cw.store.UpdateVisited(req.URL, resp.Date, resp.LastModified)
		select {
		case fc.Out <- resp:
		case <-fc.quit:
			return
		}
	}
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
		var idx, seconds int
		if idx = strings.Index(c, "s-maxage="); idx < 0 {
			idx = strings.Index(c, "max-age=")
		}
		if idx >= 0 {
			idx = strings.Index(c[idx:], "=")
			if _, err := fmt.Sscanf(c[idx+1:], "%d", &seconds); err == nil {
				resp.MaxAge = time.Duration(seconds) * time.Second
				resp.Expires = resp.Date.Add(resp.MaxAge)
				resp.Cacheable = true
			}
		}
	}

	var baseurl *url.URL
	if baseurl = resp.Request.URL; baseurl == nil {
		baseurl = resp.RequestURL
	}
	resp.NewURL = baseurl
	if l, err := resp.Location(); err == nil {
		baseurl, resp.NewURL = l, l
	}
	if l, err := baseurl.Parse(
		resp.Header.Get("Content-Location")); err == nil {
		resp.ContentLocation = l
	}

	resp.detectMIME()
	return
}

// ReadBody reads the body of response. It can be called multi-times safely.
// Response.Body will also be closed.
func (resp *Response) ReadBody(maxLen int64) error {
	if resp.Closed {
		return nil
	}
	defer resp.CloseBody()
	if resp.ContentLength > maxLen {
		return ErrContentTooLong
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

// CloseBody closes the body of response. It can be called multiply times safely.
func (resp *Response) CloseBody() {
	if resp.Closed {
		return
	}
	resp.Body.Close()
	resp.Closed = true
}

func (resp *Response) detectMIME() {
	if t := resp.Header.Get("Content-Type"); t != "" {
		resp.ContentType = t
	} else if resp.NewURL != nil || resp.ContentLocation != nil {
		var ext string
		if resp.NewURL != nil {
			ext = path.Ext(resp.NewURL.Path)
		}
		if ext == "" && resp.ContentLocation != nil {
			ext = path.Ext(resp.ContentLocation.Path)
		}
		if ext != "" {
			resp.ContentType = mime.TypeByExtension(ext)
		}
	}
}
