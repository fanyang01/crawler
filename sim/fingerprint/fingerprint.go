package fingerprint

import (
	"hash/fnv"
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
		// Avoid allocation
		s := make([][]byte, shingle)
		joined := make([][]byte, 2*shingle-1)
		space := []byte(" ")

		var i, n int
		for f := range chFeature {
			// Collect enough features
			if n < shingle {
				s[n] = []byte(f)
				if n++; n == shingle {
					goto JOIN
				}
				continue
			}
			// Shift array to produce one space
			for i = 0; i < shingle-1; i++ {
				s[i] = s[i+1]
			}
			s[i] = []byte(f)

		JOIN:
			for i, f := range s {
				joined[2*i] = f
				if i+1 != len(s) {
					joined[2*i+1] = space
				}
			}
			ch <- hash(joined...)
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

func hash(s ...[]byte) uint64 {
	h := fnv.New64()
	for _, b := range s {
		h.Write(b)
	}
	return h.Sum64()
}
