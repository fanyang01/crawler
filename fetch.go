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

	log "github.com/Sirupsen/logrus"
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
	store  URLStore
}

func (cw *Crawler) newFetcher() *fetcher {
	nworker := cw.opt.NWorker.Fetcher
	this := &fetcher{
		Out:   make(chan *Response, nworker),
		store: cw.urlStore,
		cache: newCachePool(cw.opt.MaxCacheSize),
	}
	this.nworker = nworker
	this.wg = &cw.wg
	this.quit = cw.quit
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
				log.Errorf("fetcher: %v", err)
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
		fc.store.VisitAt(req.URL, resp.Date, resp.LastModified)
		// redirect
		if resp.Locations.String() != req.URL.String() {
			fc.store.VisitAt(resp.Locations, resp.Date, resp.LastModified)
		}
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

	var baseurl *url.URL
	if baseurl = resp.Request.URL; baseurl == nil {
		baseurl = resp.RequestURL
	}
	resp.Locations = baseurl
	if l, err := resp.Location(); err == nil {
		baseurl, resp.Locations = l, l
	}
	if l, err := baseurl.Parse(resp.Header.Get("Content-Location")); err == nil {
		resp.ContentLocation = l
	}

	// Detect MIME types
	resp.detectMIME()
	return
}

// ReadBody reads the body of response. It can be called multi-times safely.
// Response.Body will also be closed.
func (resp *Response) ReadBody(maxLen int64) error {
	if resp.Ready {
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
