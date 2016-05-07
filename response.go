package crawler

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"sync"
	"time"

	"golang.org/x/text/encoding"

	"github.com/fanyang01/crawler/cache"
)

const (
	BodyStatusHeadOnly = iota
	BodyStatusReading
	BodyStatusEOF
	BodyStatusError
)

// Response contains a http response and some metadata.
// Note the body of response may be read or not, depending on
// the type of content and the size of content. Call ReadBody to
// safely read and close the body. Optionally, you can access Body
// directly but do NOT close it.
type Response struct {
	*http.Response
	URL       *url.URL
	NewURL    *url.URL
	Timestamp time.Time

	CacheControl *cache.Control

	ContentLocation *url.URL
	ContentType     string
	CertainType     bool
	Refresh         struct {
		Seconds int
		URL     *url.URL
	}

	bodyCloser io.ReadCloser
	Body       io.Reader
	BodyStatus int

	Charset        string
	CertainCharset bool
	Encoding       encoding.Encoding

	ctx   *Context
	err   error
	links []*url.URL
}

var (
	// respFreeList is a global free list for Response object.
	respFreeList = sync.Pool{
		New: func() interface{} { return new(Response) },
	}
	emptyResponse = Response{}
)

func NewResponse() *Response {
	r := respFreeList.Get().(*Response)
	return r
}

func (r *Response) Free() {
	links := r.links
	if len(links) > perPage {
		links = nil
	} else {
		links = links[:0]
	}
	// Let GC collect child objects.
	*r = emptyResponse
	r.links = links
	respFreeList.Put(r)
}

func (r *Response) Context() *Context { return r.ctx }
func (r *Response) Err() error        { return r.err }

type bodyReader struct {
	err    error
	rc     *io.ReadCloser
	status *int
}

func (br *bodyReader) Read(p []byte) (n int, err error) {
	switch *br.status {
	case BodyStatusHeadOnly:
		*br.status = BodyStatusReading
		fallthrough
	case BodyStatusReading:
		n, err = (*br.rc).Read(p)
		switch {
		case err == io.EOF:
			*br.status = BodyStatusEOF
			(*br.rc).Close()
		case err != nil:
			*br.status = BodyStatusError
			br.err = err
			(*br.rc).Close()
		}
		return
	case BodyStatusEOF:
		return 0, io.EOF
	default:
		return 0, br.err
	}
}

type bodyReadCloser struct {
	err      error
	body, rc io.ReadCloser
	closed   bool
}

func (rc *bodyReadCloser) Read(p []byte) (int, error) {
	if rc.closed {
		return 0, errors.New("read on closed reader")
	} else if rc.err != nil {
		return 0, rc.err
	} else if rc.rc != nil {
		return rc.rc.Read(p)
	}
	return rc.body.Read(p)
}
func (rc *bodyReadCloser) Close() error {
	if rc.closed {
		return nil
	}
	if rc.rc != nil {
		rc.rc.Close()
	}
	rc.body.Close()
	rc.closed = true
	return nil
}

// InitBody initializes Response.Body using given body, which is ensured
// to be closed at some point. If body is nil, embedded Response.Body is used.
func (resp *Response) InitBody(body io.ReadCloser) {
	if body == nil {
		body = resp.Response.Body
	}
	brc := &bodyReadCloser{
		body: body,
	}
	br := &bodyReader{
		rc:     &resp.bodyCloser,
		status: &resp.BodyStatus,
	}
	defer func() {
		resp.bodyCloser = brc
		resp.Body = br
	}()

	// Uncompress http compression
	// We prefer Content-Encoding than Tranfer-Encoding
	var encoding string
	if encoding = resp.Header.Get("Content-Encoding"); encoding == "" {
		if len(resp.TransferEncoding) == 0 {
			encoding = "identity"
		} else if len(resp.TransferEncoding) == 1 {
			encoding = resp.TransferEncoding[0]
		} else {
			brc.err = fmt.Errorf("too many encodings: %v", resp.TransferEncoding)
			return
		}
	}

	switch encoding {
	case "identity", "chunked":
		// Do nothing
	case "gzip":
		// TODO: Normally gzip encoding is auto-decoded by http package,
		// so this case may be needless.
		r, err := gzip.NewReader(body)
		if err != nil {
			brc.err = err
			return
		}
		brc.rc = ioutil.NopCloser(r)
	case "deflate":
		brc.rc = flate.NewReader(body)
	default:
		brc.err = fmt.Errorf("unsupported content encoding: %s", encoding)
	}
	return
}

func (resp *Response) preview(size int) ([]byte, error) {
	r := resp.Body
	preview := make([]byte, size)
	n, err := io.ReadFull(r, preview)
	switch {
	case err == io.ErrUnexpectedEOF:
		preview = preview[:n]
		r = bytes.NewReader(preview)
	case err != nil:
		return nil, err
	default:
		r = io.MultiReader(bytes.NewReader(preview), r)
	}

	resp.Body = r
	return preview, nil
}
