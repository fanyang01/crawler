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
	"path"
	"strconv"
	"strings"
	"time"
)

var (
	DefaultClient                = http.DefaultClient
	EnableUnkownLength           = true
	MaxHTMLLength          int64 = 1 << 20
	ErrTooManyEncodings          = errors.New("read response: too many encodings")
	ErrContentTooLong            = errors.New("read response: content length too long")
	ErrUnkownContentLength       = errors.New("read response: unkown content length")
)

func (w *Worker) fetch(r *Request) (resp *Response, err error) {
	if r.method == "" {
		r.method = "GET"
	}
	if r.client == nil {
		r.client = DefaultClient
	}

	req, err := http.NewRequest(r.method, r.url, bytes.NewReader(r.body))
	if err != nil {
		return
	}

	if r.config != nil {
		r.config(req)
	}
	resp = new(Response)
	resp.Response, err = r.client.Do(req)
	if err != nil {
		return
	}

	// Only status code 2xx is ok
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err = errors.New(resp.Status)
		return
	}
	resp.parseHeader()
	// Only prefetch html content
	if CT_HTML.match(resp.ContentType) {
		if err = resp.ReadBody(MaxHTMLLength); err != nil {
			return
		}
	}
	return
}

func (resp *Response) parseHeader() {
	var err error
	// Parse neccesary headers
	if t := resp.Header.Get("Time"); t != "" {
		resp.Time, err = time.Parse(http.TimeFormat, t)
		if err != nil {
			resp.Time = time.Now()
		}
	} else {
		resp.Time = time.Now()
	}
	if t := resp.Header.Get("Last-Modified"); t != "" {
		// on error, Time's zero value is used.
		resp.LastModified, _ = time.Parse(http.TimeFormat, t)
	}
	if t := resp.Header.Get("Expires"); t != "" {
		resp.Cacheable = true
		resp.Expires, _ = time.Parse(http.TimeFormat, t)
	}

	if a := resp.Header.Get("Age"); a != "" {
		if seconds, err := strconv.ParseInt(a, 0, 32); err == nil {
			resp.Age = time.Duration(seconds) * time.Second
		}
	}
	if c := resp.Header.Get("Cache-Control"); c != "" {
		if strings.HasPrefix(c, "s-maxage") {
			resp.Cacheable = true
			if seconds, err := strconv.ParseInt(
				strings.TrimPrefix(c, "s-maxage="), 0, 32); err == nil {
				resp.MaxAge = time.Duration(seconds) * time.Second
			}
		} else if strings.HasPrefix(c, "max-age") {
			resp.Cacheable = true
			if seconds, err := strconv.ParseInt(
				strings.TrimPrefix(c, "max-age="), 0, 32); err == nil {
				resp.MaxAge = time.Duration(seconds) * time.Second
			}
		}
	}
	baseurl := resp.Request.URL
	if l, err := resp.Location(); err == nil {
		baseurl, resp.Locations = l, l
	} else {
		log.Println(err)
		resp.Locations = baseurl
	}
	if l, err := baseurl.Parse(resp.Header.Get("Content-Location")); err == nil {
		resp.ContentLocation = l
	}

	// Detect MIME types
	resp.detectMIME()
	return
}

func (resp *Response) ReadBody(maxLen int64) error {
	if resp.closed {
		return nil
	}
	defer resp.closeBody()
	if resp.ContentLength > maxLen {
		return ErrContentTooLong
	}
	if resp.ContentLength < 0 && !EnableUnkownLength {
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

func (resp *Response) closeBody() {
	if resp.closed {
		return
	}
	resp.Body.Close()
	resp.closed = true
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
