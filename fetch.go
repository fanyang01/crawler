package crawler

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"io/ioutil"
	"log"
	"mime"
	"net/http"
	"path"
	"time"
)

var (
	EnableUnkownLength       = false
	MaxContentLength   int64 = 1 << 20
)

func (resp *Response) parse() (ok bool) {
	defer resp.Body.Close()
	if resp.ContentLength > MaxContentLength {
		return
	}
	if resp.ContentLength < 0 && !EnableUnkownLength {
		return
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
			log.Printf("too many encodings: %v\n", resp.TransferEncoding)
			return
		}
	}

	rc := resp.Body
	needclose := false // resp.Body.Close() is defered
	switch encoding {
	case "identity":
	case "gzip":
		r, err := gzip.NewReader(rc)
		if err != nil {
			log.Printf("uncompress: %v\n", err)
			return
		}
		rc, needclose = ioutil.NopCloser(r), true
	case "deflate":
		rc, needclose = flate.NewReader(rc), true
	default:
		log.Printf("unsupported content encoding: %s\n", encoding)
		return
	}

	var err error
	resp.Content, err = ioutil.ReadAll(rc)
	if needclose {
		rc.Close()
	}
	if err != nil {
		log.Printf("read response body: %v\n", err)
		return
	}

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
		resp.Expires, _ = time.Parse(http.TimeFormat, t)
	}
	if l, err := resp.Location(); err == nil {
		resp.Locations = *newURL(l)
	}

	// Detect MIME types
	resp.detectMIME()
	return true
}

func (r *Request) fetch() (resp *Response, err error) {
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
	return
}

func (resp *Response) detectMIME() {
	if t := resp.Header.Get("Content-Type"); t != "" {
		resp.ContentType = t
	} else if resp.Location != nil {
		if t := mime.TypeByExtension(
			path.Ext(resp.Locations.Path)); t != "" {
			resp.ContentType = t
		}
	} else {
		resp.ContentType = http.DetectContentType(resp.Content)
	}
}
