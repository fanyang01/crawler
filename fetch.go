package crawler

import (
	"bytes"
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

	"golang.org/x/net/html"

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
		var resp *Response
		var ok bool
		var err error
		// First check cache
		if resp, ok = fc.cache.Get(req.URL.String()); !ok {
			if resp, err = req.Client.Do(req); err != nil {
				logrus.Errorf("fetcher: %v", err)
				fc.ErrOut <- req.URL
				continue
			}

			fc.parse(resp)

			// Add to cache
			fc.cache.Add(resp)
		}
		// Redirected response is treated as the response of original URL
		fc.cw.store.UpdateVisitTime(req.URL, resp.Date, resp.LastModified)
		select {
		case fc.Out <- resp:
		case <-fc.quit:
			return
		}
	}
}

func (fc *fetcher) parse(resp *Response) {
	resp.parseHeader()
	if sure := resp.detectMIME(); !sure {
		if ok := fc.readBody(resp); ok {
			resp.ContentType = http.DetectContentType(resp.Content)
		}
	}
	// Prefetch html document
	if CT_HTML.match(resp.ContentType) {
		if ok := fc.readBody(resp); ok {
			resp.scanMeta(resp.Content)
		}
	}
}

func (fc *fetcher) readBody(resp *Response) (ok bool) {
	switch resp.BodyStatus {
	case RespStatusHeadOnly:
		if err := resp.ReadBody(fc.cw.opt.MaxHTML); err != nil {
			logrus.Errorf("fetcher: %v", err)
			fc.ErrOut <- resp.RequestURL
			return false
		}
		return true
	case RespStatusReady:
		return true
	default:
		return false
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
	return
}

// ReadBody reads the body of response and closes the body.
// It can be called multiply times safely.
func (resp *Response) ReadBody(maxLen int64) (err error) {
	if resp.BodyStatus != RespStatusHeadOnly {
		return resp.BodyError
	}
	defer func() {
		resp.Body.Close()
		if err != nil {
			resp.BodyStatus = RespStatusError
		} else {
			resp.BodyStatus = RespStatusReady
		}
		resp.BodyError = err
	}()

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

	resp.Content, err = ioutil.ReadAll(rc)
	if needclose {
		rc.Close()
	}
	return err
}

// CloseBody closes the body of a response. It can be called multiply times.
func (resp *Response) CloseBody() {
	if resp.BodyStatus == RespStatusHeadOnly {
		resp.Body.Close()
		resp.BodyStatus = RespStatusClosed
	}
}

func (resp *Response) detectMIME() (sure bool) {
	if t := resp.Header.Get("Content-Type"); t != "" {
		resp.ContentType = t
		return true
	}

	if resp.NewURL != nil || resp.ContentLocation != nil {
		var pth, ext string
		if resp.NewURL != nil {
			pth = resp.NewURL.Path
			ext = path.Ext(pth)
		}
		if ext == "" && resp.ContentLocation != nil {
			pth = resp.ContentLocation.Path
			ext = path.Ext(pth)
		}
		if ext != "" {
			resp.ContentType = mime.TypeByExtension(ext)
			return true
		} else if strings.HasSuffix(pth, "/") {
			resp.ContentType = "text/html"
			return true
		}
	}
	resp.ContentType = string(CT_UNKOWN)
	return false
}

func (resp *Response) scanMeta(content []byte) {
	if len(content) == 0 {
		return
	}
	if len(content) > 1024 {
		content = content[:1024]
	}
	z := html.NewTokenizer(bytes.NewReader(content))
	for {
		switch z.Next() {
		case html.ErrorToken:
			return

		case html.StartTagToken, html.SelfClosingTagToken:
			tagName, hasAttr := z.TagName()
			if !bytes.Equal(tagName, []byte("meta")) {
				continue
			}
			attrList := make(map[string]bool)

			const (
				pragmaUnknown = iota
				pragmaContentType
				pragmaRefresh
			)
			pragma := pragmaUnknown
			content := ""

			for hasAttr {
				var key, val []byte
				key, val, hasAttr = z.TagAttr()
				ks := string(key)
				if attrList[ks] {
					continue
				}
				attrList[ks] = true
				// ASCII case-insensitive
				for i, c := range val {
					if 'A' <= c && c <= 'Z' {
						val[i] = c + 0x20
					}
				}
				switch ks {
				case "http-equiv":
					switch string(val) {
					case "content-type":
						pragma = pragmaContentType
					case "refresh":
						pragma = pragmaRefresh
					}
				case "content":
					content = string(val)
				case "charset":
					if s := bytes.TrimSpace(val); len(s) > 0 {
						resp.Charset = string(s)
					}
				}
			}

			switch pragma {
			case pragmaUnknown:
				continue
			case pragmaContentType:
				if content = strings.TrimSpace(content); content != "" {
					resp.ContentType = fmtContentType(content)
				}
			case pragmaRefresh:
				if content = strings.TrimSpace(content); content != "" {
					resp.Refresh.Second, resp.Refresh.URL = parseRefresh(content, resp.NewURL)
				}
			}
		}
	}
}

func fmtContentType(s string) string {
	m, p, err := mime.ParseMediaType(s)
	if err != nil {
		return s
	}
	return mime.FormatMediaType(m, p)
}

func parseRefresh(s string, u *url.URL) (second int, uu *url.URL) {
	var i int
	var err error
	if i = strings.IndexAny(s, ";,"); i == -1 {
		second, _ = strconv.Atoi(strings.TrimRight(s, " \t\n\f\r"))
		return
	}
	if second, err = strconv.Atoi(strings.TrimRight(s[:i], " \t\n\f\r")); err != nil {
		return
	}
	s = strings.TrimLeft(s[i+1:], " \t\n\f\r")
	if i = strings.Index(s, "url"); i == -1 {
		return
	}
	s = strings.TrimLeft(s[i+len("url"):], " \t\n\f\r")
	if !strings.HasPrefix(s, "=") {
		return
	}
	s = strings.TrimLeft(s[1:], " \t\n\f\r")
	uu, _ = u.Parse(s)
	return
}
