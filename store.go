package crawler

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"net/url"
	"sync"
	"time"

	"github.com/boltdb/bolt"
	"github.com/fanyang01/crawler/bktree"
)

const (
	// Status of a URL.
	URLprocessing = iota
	URLfinished
	URLerror
)

var (
	ErrNotFound = errors.New("item is not found in store")
)

//easyjson:json
//go:generate easyjson $GOFILE
// URL contains metadata of a url in crawler.
type URL struct {
	Loc    url.URL
	Depth  int
	Status int

	// Can modified by Update
	Freq       time.Duration
	LastMod    time.Time
	LastTime   time.Time
	VisitCount int
	ErrCount   int
}

func (u *URL) clone() *URL {
	uu := *u
	return &uu
}

func (uu *URL) update(u *URL) {
	uu.ErrCount = u.ErrCount
	uu.VisitCount = u.VisitCount
	uu.LastTime = u.LastTime
	uu.LastMod = u.LastMod
}

// Store stores all URLs.
type Store interface {
	Exist(u *url.URL) (bool, error)
	Get(u *url.URL) (*URL, error)
	GetDepth(u *url.URL) (int, error)
	PutNX(u *URL) (bool, error)
	Update(u *URL) error
	UpdateStatus(u *url.URL, status int) error

	IncVisitCount() error
	IncErrorCount() error
	IsFinished() (bool, error)
}

type MemStore struct {
	sync.RWMutex
	m map[url.URL]*URL

	bktree *bktree.Tree

	URLs     int32
	Finished int32
	Ntimes   int32
	Errors   int32
}

func newMemStore() *MemStore {
	return &MemStore{
		m:      make(map[url.URL]*URL),
		bktree: bktree.New(),
	}
}

func (p *MemStore) Exist(u *url.URL) (bool, error) {
	p.RLock()
	defer p.RUnlock()
	_, ok := p.m[*u]
	return ok, nil
}

func (p *MemStore) Get(u *url.URL) (uu *URL, err error) {
	p.RLock()
	entry, present := p.m[*u]
	if present {
		uu = entry.clone()
	} else {
		err = ErrNotFound
	}
	p.RUnlock()
	return
}

func (p *MemStore) GetDepth(u *url.URL) (int, error) {
	p.RLock()
	defer p.RUnlock()
	if uu, ok := p.m[*u]; ok {
		return uu.Depth, nil
	}
	return 0, nil
}

func (p *MemStore) PutNX(u *URL) (bool, error) {
	p.Lock()
	defer p.Unlock()
	if _, ok := p.m[u.Loc]; ok {
		return false, nil
	}
	p.m[u.Loc] = u.clone()
	p.URLs++
	return true, nil
}

func (p *MemStore) Update(u *URL) error {
	p.Lock()
	defer p.Unlock()
	uu, ok := p.m[u.Loc]
	if !ok {
		return nil
	}
	uu.update(u)
	return nil
}

func (p *MemStore) UpdateStatus(u *url.URL, status int) error {
	p.Lock()
	defer p.Unlock()

	uu, ok := p.m[*u]
	if !ok {
		return nil
	}
	uu.Status = status
	switch status {
	case URLfinished, URLerror:
		p.Finished++
	}
	return nil
}

func (s *MemStore) IncVisitCount() error {
	s.Lock()
	s.Ntimes++
	s.Unlock()
	return nil
}

func (s *MemStore) IncErrorCount() error {
	s.Lock()
	s.Errors++
	s.Unlock()
	return nil
}

func (s *MemStore) IsFinished() (bool, error) {
	s.RLock()
	defer s.RUnlock()
	return s.Finished >= s.URLs, nil
}

type BoltStore struct {
	DB *bolt.DB
}

var (
	bkURL          = []byte("URL_BUCKET")
	bkCount        = []byte("CNT_BUCKET")
	keyVisitCount  = []byte("VISIT_COUNT_BUCKET")
	keyURLCount    = []byte("URL_COUNT_BUCKET")
	keyErrorCount  = []byte("ERROR_COUNT_BUCKET")
	keyFinishCount = []byte("FINISH_COUNT_BUCKET")
)

func NewBoltStore(path string, opt *bolt.Options) (bs *BoltStore, err error) {
	bs = &BoltStore{}
	if bs.DB, err = bolt.Open(path, 0644, opt); err != nil {
		return nil, err
	}
	err = bs.DB.Update(func(tx *bolt.Tx) error {
		if _, err = tx.CreateBucketIfNotExists(bkURL); err != nil {
			return err
		}
		b, err := tx.CreateBucketIfNotExists(bkCount)
		if err != nil {
			return err
		}
		for _, k := range [][]byte{
			keyVisitCount, keyURLCount, keyErrorCount, keyFinishCount,
		} {
			if _, err := bkPutNX(b, k, i64tob(0)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		bs = nil
	}
	return
}

func bkPutNX(b *bolt.Bucket, k, v []byte) (ok bool, err error) {
	if b.Get(k) != nil {
		ok = false
		return
	}
	if err = b.Put(k, v); err == nil {
		ok = true
	}
	return
}

func i64tob(i int64) []byte {
	b := make([]byte, 8)
	binary.PutVarint(b, i)
	return b
}

func btoi64(b []byte) int64 {
	i, _ := binary.Varint(b)
	return i
}

func (s *BoltStore) Exist(u *url.URL) (yes bool, err error) {
	err = s.DB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bkURL)
		if b.Get([]byte(u.String())) != nil {
			yes = true
		}
		return nil
	})
	return
}

func getFromBucket(b *bolt.Bucket, u *url.URL) (uu *URL, err error) {
	v := b.Get([]byte(u.String()))
	if v == nil {
		return nil, ErrNotFound
	}
	uu = &URL{}
	err = json.Unmarshal(v, &uu)
	return
}

func getFromTx(tx *bolt.Tx, u *url.URL) (uu *URL, err error) {
	b := tx.Bucket(bkURL)
	return getFromBucket(b, u)
}

func (s *BoltStore) Get(u *url.URL) (uu *URL, err error) {
	err = s.DB.View(func(tx *bolt.Tx) error {
		uu, err = getFromTx(tx, u)
		return err
	})
	return
}

func (s *BoltStore) GetDepth(u *url.URL) (depth int, err error) {
	err = s.DB.View(func(tx *bolt.Tx) error {
		var uu *URL
		if uu, err = getFromTx(tx, u); err == nil {
			depth = uu.Depth
		}
		return err
	})
	return
}

func (s *BoltStore) PutNX(u *URL) (ok bool, err error) {
	err = s.DB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bkURL)
		k := []byte(u.Loc.String())
		v, err := json.Marshal(u)
		if err != nil {
			return err
		}
		if ok, err = bkPutNX(b, k, v); err != nil {
			return err
		} else if !ok {
			return nil
		}

		b = tx.Bucket(bkCount)
		cnt := btoi64(b.Get(keyURLCount)) + 1
		return b.Put(keyURLCount, i64tob(cnt))
	})
	return
}

func (s *BoltStore) Update(u *URL) error {
	return s.DB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bkURL)
		uu, err := getFromBucket(b, &u.Loc)
		if err != nil {
			return err
		}
		uu.update(u)
		k := []byte(u.Loc.String())
		v, err := json.Marshal(uu)
		if err != nil {
			return err
		}
		return b.Put(k, v)
	})
}

func (s *BoltStore) UpdateStatus(u *url.URL, status int) error {
	return s.DB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bkURL)
		uu, err := getFromBucket(b, u)
		if err != nil {
			return err
		}
		uu.Status = status
		k := []byte(u.String())
		v, err := json.Marshal(uu)
		if err != nil {
			return err
		}
		if err = b.Put(k, v); err != nil {
			return err
		}
		switch status {
		case URLfinished, URLerror:
			b = tx.Bucket(bkCount)
			cnt := btoi64(b.Get(keyFinishCount)) + 1
			return b.Put(keyFinishCount, i64tob(cnt))
		}
		return nil
	})
}

func (s *BoltStore) IncVisitCount() error {
	return s.DB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bkCount)
		cnt := btoi64(b.Get(keyVisitCount)) + 1
		return b.Put(keyVisitCount, i64tob(cnt))
	})
}

func (s *BoltStore) IncErrorCount() (err error) {
	return s.DB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bkCount)
		cnt := btoi64(b.Get(keyErrorCount)) + 1
		return b.Put(keyErrorCount, i64tob(cnt))
	})
}

func (s *BoltStore) IsFinished() (is bool, err error) {
	err = s.DB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bkCount)
		finish := btoi64(b.Get(keyFinishCount))
		urlcnt := btoi64(b.Get(keyURLCount))
		is = finish >= urlcnt
		return nil
	})
	return
}
