package download

import (
	"bytes"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fanyang01/crawler/bktree"
	"github.com/fanyang01/crawler/fingerprint"
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

type SimDownloader struct {
	Dir         string
	GenPath     func(*url.URL) string
	PreDownload bool

	Distance int
	Shingle  int
	MaxToken int

	FreeList *FreeList

	bktree struct {
		sync.RWMutex
		*bktree.Tree
	}
	once sync.Once
}

func (d *SimDownloader) init() {
	d.once.Do(func() {
		if !d.PreDownload && d.FreeList == nil {
			d.FreeList = NewFreeList(32, 1<<21) // 32 * 2MB
		}
		if d.bktree.Tree == nil {
			d.bktree.Tree = bktree.New()
		}
		if d.GenPath == nil {
			d.GenPath = d.genPath
		}
		if d.MaxToken == 0 {
			d.MaxToken = 4096
		}
	})
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

func (d *SimDownloader) Handle(u *url.URL, r io.Reader) (similar bool, err error) {
	d.init()

	var f *os.File
	var buf *bytes.Buffer
	var tee io.Reader
	pth := d.GenPath(u)

	if d.PreDownload {
		if f, err = file(pth); err != nil {
			return
		}
		defer f.Close()
		tee = io.TeeReader(r, f)
	} else {
		buf = d.FreeList.Get()
		defer d.FreeList.Put(buf)
		tee = io.TeeReader(r, buf)
	}

	fp := fingerprint.Compute(tee, d.MaxToken, d.Shingle)
	if d.hasSimilar(fp, d.Distance) {
		similar = true
		if d.PreDownload {
			err = os.Remove(f.Name())
		}
		return
	}

	if _, err = io.Copy(ioutil.Discard, tee); err != nil {
		return
	} else if d.PreDownload { // content has been copy to file
		goto DONE
	} else if f, err = file(pth); err != nil {
		return
	}
	defer f.Close()

	if _, err = io.Copy(f, buf); err != nil {
		os.Remove(f.Name()) // TODO: handler error
		return
	}
DONE:
	d.addFingerprint(fp)
	return
}

func (d *SimDownloader) genPath(u *url.URL) string {
	// pth := u.Path
	pth := u.EscapedPath()
	if strings.HasSuffix(pth, "/") {
		pth += "index.html"
	} else if path.Ext(pth) == "" {
		pth += "/index.html"
	}
	if u.RawQuery != "" {
		pth += ".QUERY." + u.Query().Encode()
	}
	return filepath.Join(
		d.Dir,
		u.Host,
		filepath.FromSlash(path.Clean(pth)),
	)
}

func (d *SimDownloader) addFingerprint(f uint64) {
	d.bktree.Lock()
	d.bktree.Add(f)
	d.bktree.Unlock()
}

func (d *SimDownloader) hasSimilar(f uint64, r int) bool {
	d.bktree.RLock()
	ok := d.bktree.Has(f, r)
	d.bktree.RUnlock()
	return ok
}
