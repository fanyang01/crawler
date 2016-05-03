package boltstore

import (
	"errors"
	"net/url"

	"github.com/boltdb/bolt"
	"github.com/fanyang01/crawler"
	"github.com/fanyang01/crawler/bloom"
	"github.com/fanyang01/crawler/codec"
	"github.com/fanyang01/crawler/util"
)

type BoltStore struct {
	DB     *bolt.DB
	filter *bloom.Filter
	codec  codec.Codec
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
		filter: bloom.NewFilter(-1, 0.001),
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
	v := b.Get([]byte(u.String()))
	if v == nil {
		return nil, errors.New("boltstore: item is not found")
	}
	uu = &crawler.URL{}
	err = s.codec.Unmarshal(v, &uu)
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

func (s *BoltStore) GetDepth(u *url.URL) (depth int, err error) {
	err = s.DB.View(func(tx *bolt.Tx) error {
		var uu *crawler.URL
		if uu, err = s.getFromTx(tx, u); err == nil {
			depth = uu.Depth
		}
		return err
	})
	return
}

func (s *BoltStore) PutNX(u *crawler.URL) (ok bool, err error) {
	err = s.DB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bkURL)
		k := []byte(u.Loc.String())
		v, err := s.codec.Marshal(u)
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
		s.filter.Add(&u.Loc)
	}
	return
}

func (s *BoltStore) Update(u *crawler.URL) error {
	return s.DB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bkURL)
		uu, err := s.getFromBucket(b, &u.Loc)
		if err != nil {
			return err
		}
		uu.Update(u)
		k := []byte(u.Loc.String())
		v, err := s.codec.Marshal(uu)
		if err != nil {
			return err
		}
		return b.Put(k, v)
	})
}

func (s *BoltStore) UpdateStatus(u *url.URL, status int) error {
	return s.DB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bkURL)
		uu, err := s.getFromBucket(b, u)
		if err != nil {
			return err
		}
		uu.Status = status
		k := []byte(u.String())
		v, err := s.codec.Marshal(uu)
		if err != nil {
			return err
		}
		if err = b.Put(k, v); err != nil {
			return err
		}
		switch status {
		case crawler.URLStatusFinished, crawler.URLStatusError:
			b = tx.Bucket(bkCount)
			cnt := util.Btoi64(b.Get(keyFinishCount)) + 1
			return b.Put(keyFinishCount, util.I64tob(cnt))
		}
		return nil
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
		var u crawler.URL
		for k, v := c.First(); k != nil; k, v = c.Next() {
			if err := s.codec.Unmarshal(v, &u); err != nil {
				return err
			}
			switch u.Status {
			case crawler.URLStatusProcessing:
				ch <- &u.Loc
			}
		}
		return nil
	})
}
