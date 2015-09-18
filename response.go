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
