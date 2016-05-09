package crawler

import (
	"bytes"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/fanyang01/crawler/media"
	"github.com/fanyang01/crawler/urlx"
	"github.com/fanyang01/crawler/util"

	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
)

type fetcher struct {
	workerConn
	In     <-chan *Request
	Out    chan *Response
	ErrOut chan *Context
	cw     *Crawler
}

func (cw *Crawler) newFetcher() *fetcher {
	nworker := cw.opt.NWorker.Fetcher
	this := &fetcher{
		Out: make(chan *Response, nworker),
		cw:  cw,
	}
	cw.initWorker("fetcher", this, nworker)
	return this
}

func (f *fetcher) cleanup() { close(f.Out) }

func (f *fetcher) work() {
	for req := range f.In {
		var (
			out    = f.Out
			errOut chan *Context
			logger = f.logger.New("url", req.URL)
		)
		r, err := req.Client.Do(req)
		if err != nil {
			req.ctx.err = err
			out, errOut = nil, f.ErrOut
			logger.Error("client failed to do request", "err", err)
			goto END
		}
		logger.Info(r.Status)
		if err := f.initResponse(req, r); err != nil {
			req.ctx.err = err
			out, errOut = nil, f.ErrOut
			logger.Error("initialize response", "err", err)
			r.free()
		}
	END:
		select {
		case out <- r:
		case errOut <- req.ctx:
		case <-f.quit:
			return
		}
	}
}

func (f *fetcher) initResponse(req *Request, r *Response) error {
	// Redirected response is treated as the response of original URL,
	// because we need to ensure there is only one instance of a URL in the
	// processing flow, but many URLs can redirect to the same location.
	r.URL = req.URL
	if r.NewURL == nil {
		r.NewURL = r.URL
	}
	r.ctx = req.ctx
	r.Timestamp = time.Now()
	r.scanLocation()
	if err := r.normalize(f.cw.normalize); err != nil {
		return err
	}
	r.detectContentType()

	var (
		preview []byte
		err     error
	)
	if preview, err = r.preview(1024); err != nil {
		return fmt.Errorf("preview: %v", err)
	}
	if !r.CertainType {
		r.ContentType = http.DetectContentType(preview)
	}
	r.scanHTMLMeta(preview)
	r.convToUTF8(preview, f.cw.ctrl.Charset)
	return nil
}

func (r *Response) normalize(normalize func(*url.URL) error) error {
	if err := normalize(r.NewURL); err != nil {
		return err
	}
	if r.ContentLocation != nil {
		if err := normalize(r.ContentLocation); err != nil {
			return err
		}
	}
	if r.Refresh.URL != nil {
		if err := normalize(r.Refresh.URL); err != nil {
			return err
		}
	}
	return nil
}

func (r *Response) convToUTF8(preview []byte, query func(*url.URL) string) {
	// Convert to UTF-8
	if media.IsHTML(r.ContentType) {
		e, name, certain := charset.DetermineEncoding(
			preview, r.ContentType,
		)
		// according to charset package source, default unknown charset is windows-1252.
		if !certain && name == "windows-1252" {
			if e, name = charset.Lookup(query(r.URL)); e != nil {
				certain = true
			}
		}
		r.Charset, r.CertainCharset, r.Encoding = name, certain, e
		if name != "" && e != nil {
			r.Body, _ = util.NewUTF8Reader(name, r.Body)
		}
	}
}

func (r *Response) scanLocation() {
	var baseurl *url.URL
	if baseurl = r.NewURL; baseurl == nil {
		baseurl = r.URL
	}
	if loc := r.Header.Get("Content-Location"); loc != "" {
		r.ContentLocation, _ = urlx.ParseRef(baseurl, loc)
	}
	if s := r.Header.Get("Refresh"); s != "" {
		r.Refresh.Seconds, r.Refresh.URL = parseRefresh(s, baseurl)
	}
}

func (r *Response) detectContentType() (sure bool) {
	if r.CertainType {
		return true
	}
	defer func() { r.CertainType = sure }()

	if t := r.Header.Get("Content-Type"); t != "" {
		r.ContentType = t
		return true
	}
	if r.NewURL != nil || r.ContentLocation != nil {
		var pth, ext string
		if r.NewURL != nil {
			pth = r.NewURL.Path
			ext = path.Ext(pth)
		}
		if ext == "" && r.ContentLocation != nil {
			pth = r.ContentLocation.Path
			ext = path.Ext(pth)
		}
		if ext != "" {
			if t := mime.TypeByExtension(ext); t != "" {
				r.ContentType = t
				return true
			}
		} else if strings.HasSuffix(pth, "/") {
			r.ContentType = "text/html"
			return false
		}
	}
	r.ContentType = string(media.UNKNOWN)
	return false
}

func (r *Response) scanHTMLMeta(content []byte) {
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
						r.Charset = string(s)
						r.CertainCharset = true
					}
				}
			}

			switch pragma {
			case pragmaUnknown:
				continue
			case pragmaContentType:
				if content = strings.TrimSpace(content); content != "" {
					// Override content type.
					if !r.CertainType || contentCharset(r.ContentType) == "" {
						r.ContentType = fmtContentType(content)
						r.CertainType = true
					}
				}
			case pragmaRefresh:
				if content = strings.TrimSpace(content); content != "" {
					r.Refresh.Seconds, r.Refresh.URL = parseRefresh(content, r.NewURL)
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

func contentCharset(s string) string {
	_, p, err := mime.ParseMediaType(s)
	if err != nil {
		return s
	}
	return p["charset"]
}

func parseRefresh(s string, u *url.URL) (second int, uu *url.URL) {
	const blank = " \t\n\f\r"
	var i int
	var err error
	if i = strings.IndexAny(s, ";,"); i == -1 {
		second, _ = strconv.Atoi(strings.TrimRight(s, blank))
		return
	}
	if second, err = strconv.Atoi(strings.TrimRight(s[:i], blank)); err != nil {
		return
	}
	s = strings.TrimLeft(s[i+1:], blank)
	if i = strings.Index(s, "url"); i == -1 {
		return
	}
	s = strings.TrimLeft(s[i+len("url"):], blank)
	if !strings.HasPrefix(s, "=") {
		return
	}
	s = strings.TrimLeft(s[1:], blank)
	uu, _ = urlx.ParseRef(u, s)
	return
}
