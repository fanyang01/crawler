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
		if resp, ok = fc.cache.Get(req.URL); !ok {
			if resp, err = req.Client.Do(req); err != nil {
				logrus.Errorf("fetcher: %v", err)
				fc.ErrOut <- req.URL
				continue
			}
			if resp.NewURL != req.URL {
				// TODO
				rr := newResponse()
				rr.NewURL = req.URL
				rr.BodyStatus = RespStatusReady
				rr.RedirectURL = resp.NewURL
				fc.Out <- rr
			}
			resp.Timestamp = time.Now()
			fc.parse(resp)

			// TODO: move to somewhere resp.Content would not change.
			// fc.cache.Set(resp)
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
	resp.parseCache()
	resp.parseLocation()

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

func (resp *Response) parseLocation() {
	var baseurl *url.URL
	if baseurl = resp.Request.URL; baseurl == nil {
		baseurl = resp.RequestURL
	}
	resp.NewURL = baseurl
	if l, err := resp.Location(); err == nil {
		baseurl, resp.NewURL = l, l
	}
	if l, err := baseurl.Parse(resp.Header.Get("Content-Location")); err == nil {
		resp.ContentLocation = l
	}
	if s := resp.Header.Get("Refresh"); s != "" {
		resp.Refresh.Seconds, resp.Refresh.URL = parseRefresh(s, resp.NewURL)
	}
}

// https://www.w3.org/Protocols/rfc2616/rfc2616-sec13.html
func (resp *Response) parseCache() {
	var date time.Time
	var err error
	if t := resp.Header.Get("Date"); t != "" {
		if date, err = time.Parse(http.TimeFormat, t); err != nil {
			date = resp.Timestamp
		}
	}
	resp.Date = date

	var maxAge time.Duration
	kv := map[string]string{}
	if c := resp.Header.Get("Cache-Control"); c != "" {
		kv = parseCacheControl(c)
		var sec int64
		if v, ok := kv["max-age"]; ok {
			if i, err := strconv.ParseInt(v, 0, 32); err != nil {
				sec = i
			}
		}
		if v, ok := kv["s-maxage"]; ok {
			if i, err := strconv.ParseInt(v, 0, 32); err == nil && i > sec {
				sec = i
			}
		}
		maxAge = time.Duration(sec) * time.Second
		if maxAge == 0 {
			if t := resp.Header.Get("Expires"); t != "" {
				expire, err := time.Parse(http.TimeFormat, t)
				if err == nil && !date.IsZero() {
					maxAge = expire.Sub(date)
				}
			}
		}
	}
	resp.MaxAge = maxAge

	switch resp.StatusCode {
	case 200, 203, 206, 300, 301:
		// Do nothing
	default:
		resp.CacheType = CacheDisallow
		return
	}
	exist := func(directive string) bool {
		_, ok := kv[directive]
		return ok
	}
	switch {
	case exist("no-store"):
		fallthrough
	default:
		resp.CacheType = CacheDisallow
		return
	case exist("must-revalidate") || exist("no-cache"):
		resp.CacheType = CacheNeedValidate
	case maxAge != 0:
		resp.CacheType = CacheNormal
	}

	var age time.Duration
	if a := resp.Header.Get("Age"); a != "" {
		if seconds, err := strconv.ParseInt(a, 0, 32); err == nil {
			age = time.Duration(seconds) * time.Second
		}
	}
	resp.Age = computeAge(date, resp.Timestamp, age)

	resp.ETag = resp.Header.Get("ETag")
	if t := resp.Header.Get("Last-Modified"); t != "" {
		resp.LastModified, _ = time.Parse(http.TimeFormat, t)
	}
}

func max64(x, y time.Duration) time.Duration {
	if x > y {
		return x
	}
	return y
}

// Use a simplied calculation of rfc2616-sec13.
func computeAge(date, resp time.Time, age time.Duration) time.Duration {
	apparent := max64(0, resp.Sub(date))
	recv := max64(apparent, age)
	// assume delay = 0
	// initial := recv + delay
	resident := time.Now().Sub(resp)
	return recv + resident
}

func (resp *Response) IsExpired() bool {
	age := computeAge(resp.Date, resp.Timestamp, resp.Age)
	if age > resp.MaxAge {
		return true
	}
	return false
}

func (resp *Response) IsCacheable() bool {
	switch resp.CacheType {
	case CacheNeedValidate, CacheNormal:
		return true
	}
	return false
}

func parseCacheControl(s string) (kv map[string]string) {
	kv = make(map[string]string)
	parts := strings.Split(strings.TrimSpace(s), ",")
	if len(parts) == 1 && parts[0] == "" {
		return
	}
	for i := 0; i < len(parts); i++ {
		parts[i] = strings.TrimSpace(parts[i])
		if len(parts[i]) == 0 {
			continue
		}
		name, val := parts[i], ""
		if j := strings.Index(name, "="); j >= 0 {
			name = strings.TrimRight(name[:j], " \t\r\n\f")
			val = strings.TrimLeft(name[j+1:], " \t\r\n\f")
			if len(val) > 0 {
				kv[name] = val
			}
			continue
		}
		kv[name] = ""
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
		// Do nothing
	case "gzip":
		// TODO: Normally gzip encoding is auto-decoded by http package,
		// so this case may be needless.
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
					resp.Refresh.Seconds, resp.Refresh.URL = parseRefresh(content, resp.NewURL)
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
