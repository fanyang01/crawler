package crawler

import (
	"compress/flate"
	"compress/gzip"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"mime"
	"net"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"
)

const (
	// MaxHTMLLen is the max size of a html file, which is 1MB here.
	MaxHTMLLen = 1 << 20
)

var (
	DefaultHTTPTransport = &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		Dial: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).Dial,
		TLSHandshakeTimeout: 5 * time.Second,
	}
	DefaultHTTPClient = &http.Client{
		Timeout:   time.Second * 5,
		Transport: DefaultHTTPTransport,
	}

	// DefaultClient caches cachalbe content and limits the size of html file.
	DefaultClient = &StdClient{
		Client:          DefaultHTTPClient,
		MaxHTMLLen:      MaxHTMLLen,
		EnableUnkownLen: true,
	}
)

// StdClient is a client for crawling static pages.
type StdClient struct {
	Client          *http.Client
	MaxHTMLLen      int64
	EnableUnkownLen bool
}

// Do implements Client.
func (ct *StdClient) Do(req *Request) (resp *Response, err error) {
	resp = &Response{}
	resp.RequestURL = req.URL
	resp.Response, err = ct.Client.Do(req.Request)
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
		baseurl = resp.RequestURL
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

// ReadBody reads the body of response. It can be called multi-times safely.
// Response.Body will also be closed.
func (resp *Response) ReadBody(maxLen int64, enableUnkownLen bool) error {
	if resp.ready {
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

// CloseBody closes the body of response. It can be called multi-times safely.
func (resp *Response) CloseBody() {
	if resp.ready {
		return
	}
	resp.Body.Close()
	resp.ready = true
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
