package util

import (
	"bytes"
	"io/ioutil"

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
