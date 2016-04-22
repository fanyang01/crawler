package fingerprint

import (
	"bytes"
	"io"

	"github.com/mfonda/simhash"

	"golang.org/x/net/html"
)

func Compute(r io.Reader, N, shingle int) uint64 {
	if shingle < 1 {
		shingle = 1
	}
	chFeature := make(chan string, 128)
	go func() {
		z := html.NewTokenizer(r)
		count := 1
		for tt := z.Next(); count < N && tt != html.ErrorToken; tt = z.Next() {
			t := z.Token()
			count++
			genFeature(&t, chFeature)
		}
		close(chFeature)
	}()

	ch := make(chan uint64, 128)
	go func() {
		s := make([][]byte, shingle)
		var i, n int
		for f := range chFeature {
			if n < shingle {
				s[n] = []byte(f)
				n++
				continue
			}
			for i = 0; i < shingle-1; i++ {
				s[i] = s[i+1]
			}
			s[i] = []byte(f)
			ch <- simhash.NewFeature(bytes.Join(s, []byte(" "))).Sum()
		}
		close(ch)
	}()

	v := simhash.Vector{}
	var i uint
	var bit int
	for n := range ch {
		for i = 0; i < 64; i++ {
			bit = int((n >> i) & 1)
			// bit == 1 ? 1 : -1
			v[i] += (bit ^ (bit - 1))
		}
	}
	return simhash.Fingerprint(v)
}

func genFeature(t *html.Token, ch chan<- string) {
	var s string
	switch t.Type {
	// case html.ErrorToken:
	case html.StartTagToken:
		s = "A:" + t.DataAtom.String()
	case html.EndTagToken:
		s = "B:" + t.DataAtom.String()
	case html.SelfClosingTagToken:
		s = "C:" + t.DataAtom.String()
	case html.DoctypeToken:
		s = "D:" + t.DataAtom.String()
	case html.CommentToken:
		s = "E:" + t.DataAtom.String()
	case html.TextToken: // TODO
		s = "F:" + t.DataAtom.String()
	}
	ch <- s

	for _, attr := range t.Attr {
		switch attr.Key {
		case "class", "href", "name", "rel":
			s = "G:" + t.DataAtom.String() + ":" + attr.Key + ":" + attr.Val
		default:
			s = "G:" + t.DataAtom.String() + ":" + attr.Key
		}
	}
	ch <- s
}
