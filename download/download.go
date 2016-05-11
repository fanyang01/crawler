package download

import (
	"bytes"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type FreeList struct {
	ch        chan *bytes.Buffer
	threshold int
}

func NewFreeList(size, threshold int) *FreeList {
	return &FreeList{
		ch:        make(chan *bytes.Buffer, size),
		threshold: threshold,
	}
}

func (f *FreeList) Get() *bytes.Buffer {
	select {
	case b := <-f.ch:
		b.Reset()
		return b
	default:
		return new(bytes.Buffer)
	}
}

func (f *FreeList) Put(b *bytes.Buffer) {
	if b.Cap() > f.threshold {
		return
	}
	select {
	case f.ch <- b:
	default:
	}
}

type Downloader struct {
	Dir     string
	GenPath func(*url.URL) string
}

func file(pth string) (f *os.File, err error) {
	dir, _ := filepath.Split(pth)
	if err = os.MkdirAll(dir, 0755); err != nil {
		return
	}
	return os.OpenFile(
		pth,
		os.O_WRONLY|os.O_CREATE|os.O_EXCL,
		0644,
	)
}

func (d *Downloader) Handle(u *url.URL, r io.Reader) error {
	var (
		f   *os.File
		pth string
		err error
	)

	if d.GenPath != nil {
		pth = d.GenPath(u)
	} else {
		pth = d.genPath(u)
	}

	if f, err = file(pth); err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, r)
	return err

}

func (d *Downloader) genPath(u *url.URL) string {
	pth := u.EscapedPath()
	if strings.HasSuffix(pth, "/") {
		pth += "index.html"
	} else if path.Ext(pth) == "" {
		pth += "/index.html"
	}
	if u.RawQuery != "" {
		pth += "?" + u.Query().Encode()
	}
	return filepath.Join(
		d.Dir,
		u.Host,
		filepath.FromSlash(path.Clean(pth)),
	)
}
