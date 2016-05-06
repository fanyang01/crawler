// Package media provides methods to identify media type using HTTP
// Content-Type header.
package media

import "mime"

type Type string

const (
	HTML    Type = "text/html"
	XML     Type = "text/xml"
	XHTML   Type = "application/xhtml+xml"
	PLAIN   Type = "text/plain"
	CSS     Type = "text/css"
	JS      Type = "application/javascript" // x-javascript
	JSON    Type = "application/json"
	GIF     Type = "image/gif"
	PNG     Type = "image/png"
	JPEG    Type = "image/jpeg"
	BMP     Type = "image/bmp"
	WEBP    Type = "image/webp"
	ZIP     Type = "application/zip"
	GZIP    Type = "application/x-gzip"
	RAR     Type = "application/x-rar-compressed"
	PDF     Type = "application/pdf"
	PS      Type = "application/postscript"
	OGG     Type = "application/ogg"
	WAVE    Type = "audio/wave"
	WEBM    Type = "video/webm"
	UNKNOWN Type = "application/octet-stream"
)

func (t Type) Match(header string) bool {
	m, _, err := mime.ParseMediaType(header)
	return err == nil && m == string(t)
}

func IsHTML(header string) bool {
	return HTML.Match(header) || XHTML.Match(header)
}
