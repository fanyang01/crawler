package boltstore

import (
	"errors"
	"net/url"
	"time"

	"github.com/boltdb/bolt"
	"github.com/fanyang01/crawler"
	"github.com/fanyang01/crawler/bloom"
	"github.com/fanyang01/crawler/codec"
	"github.com/fanyang01/crawler/urlx"
	"github.com/fanyang01/crawler/util"
)

type BoltStore struct {
	DB     *bolt.DB
	filter *bloom.Filter
	codec  codec.Codec
}

type wrapper struct {
	URL      string
	Depth    int
	Done     bool
	Last     time.Time
	Status   int
	NumVisit int
	NumRetry int
}

func (w *wrapper) To(url string) (*crawler.URL, error) {
	uu, err := urlx.Parse(url)
	if err != nil {
		return nil, err
	}
	u := &crawler.URL{}
	u.URL = *uu
	u.Depth = w.Depth
	u.Done = w.Done
	u.Last = w.Last
	u.Status = w.Status
	u.NumVisit = w.NumVisit
	u.NumRetry = w.NumRetry
	return u, nil
}
func (w *wrapper) From(u *crawler.URL) *wrapper {
	w.Depth = u.Depth
	w.Done = u.Done
	w.Last = u.Last
	w.Status = u.Status
	w.NumVisit = u.NumVisit
	w.NumRetry = u.NumRetry
	return w
}

var (
	bkURL          = []byte("URL_BUCKET")
	bkCount        = []byte("CNT_BUCKET")
	keyVisitCount  = []byte("VISIT_COUNT_BUCKET")
	keyURLCount    = []byte("URL_COUNT_BUCKET")
	keyErrorCount  = []byte("ERROR_COUNT_BUCKET")
	keyFinishCount = []byte("FINISH_COUNT_BUCKET")
)

func New(path string, opt *bolt.Options, e codec.Codec) (bs *BoltStore, err error) {
	if e == nil {
		e = codec.JSON
	}
	bs = &BoltStore{
		filter: bloom.NewFilter(-1, 0.0001),
		codec:  e,
	}
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
			if _, err := bkPutNX(b, k, util.I64tob(0)); err != nil {
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

func (s *BoltStore) Exist(u *url.URL) (yes bool, err error) {
	// err = s.DB.View(func(tx *bolt.Tx) error {
	// 	b := tx.Bucket(bkURL)
	// 	if b.Get([]byte(u.String())) != nil {
	// 		yes = true
	// 	}
	// 	return nil
	// })
	return s.filter.Exist(u), nil
}

func (s *BoltStore) getFromBucket(b *bolt.Bucket, u *url.URL) (uu *crawler.URL, err error) {
	us := u.String()
	v := b.Get([]byte(us))
	if v == nil {
		return nil, errors.New("boltstore: item is not found")
	}
	w := &wrapper{}
	err = s.codec.Unmarshal(v, w)
	if err == nil {
		uu, err = w.To(us)
	}
	return
}

func (s *BoltStore) getFromTx(tx *bolt.Tx, u *url.URL) (uu *crawler.URL, err error) {
	b := tx.Bucket(bkURL)
	return s.getFromBucket(b, u)
}

func (s *BoltStore) Get(u *url.URL) (uu *crawler.URL, err error) {
	err = s.DB.View(func(tx *bolt.Tx) error {
		uu, err = s.getFromTx(tx, u)
		return err
	})
	return
}

func (s *BoltStore) GetFunc(u *url.URL, f func(*crawler.URL)) error {
	return s.DB.View(func(tx *bolt.Tx) error {
		uu, err := s.getFromTx(tx, u)
		if err != nil {
			return err
		}
		f(uu)
		return nil
	})
}

func (s *BoltStore) GetDepth(u *url.URL) (depth int, err error) {
	err = s.GetFunc(u, func(uu *crawler.URL) {
		depth = uu.Depth
	})
	return
}

func (s *BoltStore) PutNX(u *crawler.URL) (ok bool, err error) {
	err = s.DB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bkURL)
		k := []byte(u.URL.String())
		w := &wrapper{}
		v, err := s.codec.Marshal(w.From(u))
		if err != nil {
			return err
		}
		if ok, err = bkPutNX(b, k, v); err != nil {
			return err
		} else if !ok {
			return nil
		}

		b = tx.Bucket(bkCount)
		cnt := util.Btoi64(b.Get(keyURLCount)) + 1
		return b.Put(keyURLCount, util.I64tob(cnt))
	})
	if err == nil && ok {
		s.filter.Add(&u.URL)
	}
	return
}

func (s *BoltStore) UpdateFunc(u *url.URL, f func(*crawler.URL)) error {
	return s.DB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bkURL)
		uu, err := s.getFromBucket(b, u)
		if err != nil {
			return err
		}
		f(uu)
		k := []byte(u.String())
		w := &wrapper{}
		v, err := s.codec.Marshal(w.From(uu))
		if err != nil {
			return err
		}
		return b.Put(k, v)
	})
}

func (s *BoltStore) Update(u *crawler.URL) error {
	return s.UpdateFunc(&u.URL, func(uu *crawler.URL) {
		uu.Update(u)
	})
}

func (s *BoltStore) Complete(u *url.URL) error {
	return s.DB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bkURL)
		uu, err := s.getFromBucket(b, u)
		if err != nil {
			return err
		}
		uu.Done = true
		k := []byte(u.String())
		w := &wrapper{}
		v, err := s.codec.Marshal(w.From(uu))
		if err != nil {
			return err
		}
		if err = b.Put(k, v); err != nil {
			return err
		}
		b = tx.Bucket(bkCount)
		cnt := util.Btoi64(b.Get(keyFinishCount)) + 1
		return b.Put(keyFinishCount, util.I64tob(cnt))
	})
}

func (s *BoltStore) IncVisitCount() error {
	return s.DB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bkCount)
		cnt := util.Btoi64(b.Get(keyVisitCount)) + 1
		return b.Put(keyVisitCount, util.I64tob(cnt))
	})
}

func (s *BoltStore) IncErrorCount() (err error) {
	return s.DB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bkCount)
		cnt := util.Btoi64(b.Get(keyErrorCount)) + 1
		return b.Put(keyErrorCount, util.I64tob(cnt))
	})
}

func (s *BoltStore) IsFinished() (is bool, err error) {
	err = s.DB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bkCount)
		finish := util.Btoi64(b.Get(keyFinishCount))
		urlcnt := util.Btoi64(b.Get(keyURLCount))
		is = finish >= urlcnt
		return nil
	})
	return
}

func (s *BoltStore) Recover(ch chan<- *url.URL) error {
	return s.DB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bkURL)
		c := b.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			w := &wrapper{}
			if err := s.codec.Unmarshal(v, w); err != nil {
				return err
			} else if w.Done {
				continue
			} else if u, err := urlx.Parse(string(k)); err != nil {
				return err
			} else {
				ch <- u
			}
		}
		return nil
	})
}

// TODO: write the bloom filter to a bucket.
func (s *BoltStore) Close() error { return s.DB.Close() }
