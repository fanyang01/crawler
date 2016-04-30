package crawler

import (
	"bytes"
	"errors"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"

	"golang.org/x/net/html"
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
	cw     *Crawler
}

func (cw *Crawler) newFetcher() *fetcher {
	nworker := cw.opt.NWorker.Fetcher
	this := &fetcher{
		Out: make(chan *Response, nworker),
		cw:  cw,
	}
	cw.initWorker(this, nworker)
	return this
}

func (fc *fetcher) cleanup() { close(fc.Out) }

func (fc *fetcher) work() {
	for req := range fc.In {
		ctx := log.WithFields(logrus.Fields{
			"worker": "fetcher",
			"URL":    req.URL.String(),
		})

		var resp *Response
		var err error
		if resp, err = req.Client.Do(req); err != nil {
			ctx.Error("client:", err)
			fc.ErrOut <- req.URL
			continue
		}
		// Redirected response is treated as the response of original
		// URL, because we need to ensure there is only one instance of
		// req.URL is in processing flow.
		resp.URL = req.URL
		if resp.Timestamp.IsZero() {
			resp.Timestamp = time.Now()
		}
		resp.parseLocation()
		resp.detectMIME()

		var preview []byte
		if preview, err = resp.preview(1024); err != nil {
			ctx.Error("preview:", err)
			fc.ErrOut <- req.URL
			continue
		}
		if !resp.CertainType {
			resp.ContentType = http.DetectContentType(preview)
		}
		resp.scanMeta(preview)
		resp.pview = preview

		select {
		case fc.Out <- resp:
		case <-fc.quit:
			return
		}
	}
}

func (resp *Response) parseLocation() {
	var baseurl *url.URL
	if baseurl = resp.Request.URL; baseurl == nil {
		if baseurl, _ = resp.Location(); baseurl == nil {
			baseurl = resp.URL
		}
	}
	if loc := resp.Header.Get("Content-Location"); loc != "" {
		resp.ContentLocation, _ = baseurl.Parse(loc)
	}
	if s := resp.Header.Get("Refresh"); s != "" {
		resp.Refresh.Seconds, resp.Refresh.URL = parseRefresh(s, baseurl)
	}
}

func (resp *Response) detectMIME() (sure bool) {
	defer func() { resp.CertainType = sure }()
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
	resp.ContentType = string(CT_UNKNOWN)
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
