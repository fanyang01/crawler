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

type rHandler interface {
	Handle(*Response)
}

type htmlHandler struct {
	ch chan *Doc
}

func (h htmlHandler) Handle(resp *Response) {
	if match := CT_HTML.match(resp.ContentType); !match {
		return
	}
	h.ch <- newDoc(resp)
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

type closeHandler struct{}

func (c closeHandler) Handle(resp *Response) {
	resp.closeBody()
}

type RespHandler struct {
	parser htmlHandler
	closer closeHandler
}

func NewRespHandler() *RespHandler {
	return &RespHandler{
		parser: htmlHandler{
			ch: make(chan *Doc),
		},
	}
}

func (h *RespHandler) Handle(in <-chan *Response, handlers ...rHandler) {
	for resp := range in {
		go func(r *Response) {
			h.parser.Handle(r)
			for _, handler := range handlers {
				handler.Handle(r)
			}
			h.closer.Handle(r)
		}(resp)
	}
}
