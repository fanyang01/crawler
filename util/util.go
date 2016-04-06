package util

import (
	"bytes"
	"io"
	"io/ioutil"

	"golang.org/x/net/html/charset"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

func ConvToUTF8(b []byte, e encoding.Encoding) (result []byte, err error) {
	reader := transform.NewReader(bytes.NewReader(b), unicode.BOMOverride(e.NewDecoder()))
	return ioutil.ReadAll(reader)
}

func ConvTo(b []byte, e encoding.Encoding) (result []byte, err error) {
	w := new(bytes.Buffer)
	writer := transform.NewWriter(w, e.NewEncoder())
	defer writer.Close()

	if _, err = writer.Write(b); err != nil {
		return
	}
	return w.Bytes(), nil
}

// From https://groups.google.com/forum/#!topic/golang-nuts/eex1wLCvK58
var boms = map[string][]byte{
	"utf-16be": []byte{0xfe, 0xff},
	"utf-16le": []byte{0xff, 0xfe},
	"utf-8":    []byte{0xef, 0xbb, 0xbf},
}

func TrimBOM(b []byte, encoding string) []byte {
	bom := boms[encoding]
	if bom != nil {
		b = bytes.TrimPrefix(b, bom)
	}
	return b
}

func NewTrimBOMReader(r io.Reader, contentType string) (io.Reader, error) {
	preview := make([]byte, 1024)
	n, err := io.ReadFull(r, preview)
	switch {
	case err == io.ErrUnexpectedEOF:
		preview = preview[:n]
		r = bytes.NewReader(preview)
	case err != nil:
		return nil, err
	}

	e, name, _ := charset.DetermineEncoding(preview, contentType)
	bom := boms[name]
	if bom != nil {
		preview = bytes.TrimPrefix(preview, bom)
	}
	r = io.MultiReader(bytes.NewReader(preview), r)
	if e != encoding.Nop {
		r = transform.NewReader(r, e.NewDecoder())
	}
	return r, nil
}
