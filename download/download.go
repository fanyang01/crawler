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

	"github.com/Sirupsen/logrus"

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

type Mirror struct {
	BufPool   *FreeList
	TargetDir string
	bktree    struct {
		sync.RWMutex
		*bktree.Tree
	}
	PreDownload bool
	once        sync.Once
}

func (m *Mirror) init() {
	m.once.Do(func() {
		if !m.PreDownload && m.BufPool == nil {
			m.BufPool = NewFreeList(32, 1<<21) // 32 * 2MB
		}
		if m.bktree.Tree == nil {
			m.bktree.Tree = bktree.New()
		}
	})
}

func (m *Mirror) Handle(u *url.URL, r io.Reader) {
	m.init()
	ctx := logrus.WithFields(logrus.Fields{
		"URL":  u.String(),
		"func": "Mirror.Handle",
	})

	buf := m.BufPool.Get()
	defer m.BufPool.Put(buf)
	tee := io.TeeReader(r, buf)

	fp := fingerprint.Compute(tee, 1<<11, 2)
	if m.hasSimiliar(fp, 3) {
		return
	}
	m.addFingerprint(fp)
	if _, err := io.Copy(ioutil.Discard, tee); err != nil {
		ctx.Error(err)
		return
	}

	pth := m.genPath(u)
	dir, _ := filepath.Split(pth)
	if err := os.MkdirAll(dir, 0755); err != nil {
		ctx.Error(err)
		return
	}
	f, err := os.OpenFile(
		pth,
		os.O_WRONLY|os.O_CREATE|os.O_EXCL,
		0644,
	)
	if err != nil {
		ctx.Error(err)
		return
	}
	defer f.Close()

	if _, err = io.Copy(f, buf); err != nil {
		ctx.Error(err)
		os.Remove(f.Name())
	}
}

func (m *Mirror) genPath(u *url.URL) string {
	pth := u.EscapedPath() // TODO: use Path?
	if strings.HasSuffix(pth, "/") {
		pth += "index.html"
	} else if path.Ext(pth) == "" {
		pth += ".html"
	}
	if u.RawQuery != "" {
		pth += ".query." + u.Query().Encode()
	}
	return filepath.Join(
		u.Host,
		filepath.FromSlash(path.Clean(pth)),
	)
}

func (m *Mirror) addFingerprint(f uint64) {
	m.bktree.Lock()
	m.bktree.Add(f)
	m.bktree.Unlock()
}

func (m *Mirror) hasSimiliar(f uint64, d int) bool {
	m.bktree.RLock()
	ok := m.bktree.Has(f, d)
	m.bktree.RUnlock()
	return ok
}
