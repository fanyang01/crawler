package crawler

import "strings"

type ContentType string

const (
	CT_HTML   ContentType = "text/html"
	CT_XML    ContentType = "text/xml"
	CT_PLAIN  ContentType = "text/plain"
	CT_CSS    ContentType = "text/css"
	CT_JS     ContentType = "application/javascript" // x-javascript
	CT_GIF    ContentType = "image/gif"
	CT_PNG    ContentType = "image/png"
	CT_JPEG   ContentType = "image/jpeg"
	CT_BMP    ContentType = "image/bmp"
	CT_WEBP   ContentType = "image/webp"
	CT_ZIP    ContentType = "application/zip"
	CT_GZIP   ContentType = "application/x-gzip"
	CT_RAR    ContentType = "application/x-rar-compressed"
	CT_PDF    ContentType = "application/pdf"
	CT_PS     ContentType = "application/postscript"
	CT_OGG    ContentType = "application/ogg"
	CT_WAVE   ContentType = "audio/wave"
	CT_WEBM   ContentType = "video/webm"
	CT_UNKOWN ContentType = "application/octet-stream"
)

func (ct ContentType) match(s string) bool {
	return strings.HasPrefix(s, string(ct))
}
