// Package util provides useful helper functions.
package util

import (
	"bytes"
	"encoding/binary"
	"fmt"
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

/*
// naive implemntation
func NewUTF8Reader(label string, r io.Reader) (io.Reader, error) {
	e, name := charset.Lookup(label)
	if e == nil {
		return nil, fmt.Errorf("unsupported charset: %q", label)
	}
	// TODO: implement custom mulitreader to use a freelist?
	preview := make([]byte, 512)
	n, err := io.ReadFull(r, preview)
	switch {
	case err == io.ErrUnexpectedEOF:
		preview = TrimBOM(preview[:n], name)
		r = bytes.NewReader(preview)
	case err != nil:
		return nil, err
	default:
		preview = TrimBOM(preview, name)
		r = io.MultiReader(bytes.NewReader(preview), r)
	}
	return transform.NewReader(r, e.NewDecoder()), nil
}
*/

func NewUTF8Reader(label string, r io.Reader) (io.Reader, error) {
	e, _ := charset.Lookup(label)
	if e == nil {
		return nil, fmt.Errorf("unsupported charset: %q", label)
	}
	return transform.NewReader(r, unicode.BOMOverride(e.NewDecoder())), nil
}

func DumpReader(r io.Reader, n int) (reader []io.Reader, done <-chan struct{}) {
	var writer []io.Writer
	for i := 0; i < n; i++ {
		r, w := io.Pipe()
		reader = append(reader, r)
		writer = append(writer, w)
	}
	ch := make(chan struct{}, 1)
	go func() {
		io.Copy(io.MultiWriter(writer...), r)
		ch <- struct{}{}
		for i := 0; i < n; i++ {
			writer[i].(*io.PipeWriter).Close()
		}
	}()
	return reader, ch
}

func I64tob(i int64) []byte {
	b := make([]byte, 8)
	binary.PutVarint(b, i)
	return b
}

func Btoi64(b []byte) int64 {
	i, _ := binary.Varint(b)
	return i
}

// func Token(z *Tokenizer) *Token {
// 	t := Token{Type: z.tt}
// 	switch z.tt {
// 	case TextToken, CommentToken, DoctypeToken:
// 		t.Data = string(z.Text())
// 	case StartTagToken, SelfClosingTagToken, EndTagToken:
// 		name, moreAttr := z.TagName()
// 		for moreAttr {
// 			var key, val []byte
// 			key, val, moreAttr = z.TagAttr()
// 			t.Attr = append(t.Attr, Attribute{"", atom.String(key), string(val)})
// 		}
// 		if a := atom.Lookup(name); a != 0 {
// 			t.DataAtom, t.Data = a, a.String()
// 		} else {
// 			t.DataAtom, t.Data = 0, string(name)
// 		}
// 	}
// 	return t
// }
