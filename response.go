package crawler

import "strings"

type ContentType string

const (
	CT_HTML   ContentType = "text/html"
	CT_XML                = "text/xml"
	CT_PLAIN              = "text/plain"
	CT_CSS                = "text/css"
	CT_JS                 = "application/javascript" // x-javascript
	CT_GIF                = "image/gif"
	CT_PNG                = "image/png"
	CT_JPEG               = "image/jpeg"
	CT_BMP                = "image/bmp"
	CT_WEBP               = "image/webp"
	CT_ZIP                = "application/zip"
	CT_GZIP               = "application/x-gzip"
	CT_RAR                = "application/x-rar-compressed"
	CT_PDF                = "application/pdf"
	CT_PS                 = "application/postscript"
	CT_OGG                = "application/ogg"
	CT_WAVE               = "audio/wave"
	CT_WEBM               = "video/webm"
	CT_UNKOWN             = "application/octet-stream"
)

func (ct ContentType) match(s string) bool {
	return strings.HasPrefix(s, string(ct))
}

type RHandler interface {
	Handle(*Response)
}

func newDoc(resp *Response) *Doc {
	doc := &Doc{
		SecondURL:   resp.ContentLocation,
		Content:     resp.Content,
		ContentType: resp.ContentType,
		Time:        resp.Time,
		Expires:     resp.Expires,
	}
	doc.Loc = resp.Locations
	doc.LastModified = resp.LastModified
	// HTTP prefer max-age than expires
	if resp.Cacheable && resp.MaxAge != 0 {
		doc.Expires = doc.Time.Add(resp.MaxAge)
	}
	return doc
}

type respHandler struct {
	option *Option
	In     chan *Response
	Out    chan *Doc
}

func newRespHandler(opt *Option) *respHandler {
	return &respHandler{
		Out:    make(chan *Doc, opt.RespHandler.OutQueueLen),
		option: opt,
	}
}

func (h *respHandler) Start(handlers ...RHandler) {
	go func() {
		for resp := range h.In {
			go func(r *Response) {
				if match := CT_HTML.match(resp.ContentType); !match {
					return
				}
				h.Out <- newDoc(resp)
				for _, handler := range handlers {
					handler.Handle(r)
				}
				resp.closeBody()
			}(resp)
		}
		close(h.Out)
	}()
}
