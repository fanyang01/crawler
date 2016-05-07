package levelstore

import (
	"net/url"

	"github.com/fanyang01/crawler"
	"github.com/fanyang01/crawler/codec"
	"github.com/fanyang01/crawler/util"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

var (
	keyVisitCount  = []byte("VISIT_COUNT")
	keyURLCount    = []byte("URL_COUNT")
	keyErrorCount  = []byte("ERROR_COUNT")
	keyFinishCount = []byte("FINISH_COUNT")
)

type LevelStore struct {
	DB    *leveldb.DB
	codec codec.Codec
}

func New(path string, o *opt.Options, e codec.Codec) (s *LevelStore, err error) {
	db, err := leveldb.OpenFile(path, o)
	if err != nil {
		return
	}
	for _, k := range [][]byte{
		keyVisitCount, keyURLCount, keyErrorCount, keyFinishCount,
	} {
		var has bool
		if has, err = db.Has(k, nil); err != nil {
			return
		} else if has {
			continue
		}
		if err = db.Put(k, util.I64tob(0), nil); err != nil {
			return
		}
	}
	if e == nil {
		e = codec.JSON
	}
	return &LevelStore{
		DB:    db,
		codec: e,
	}, nil
}

func keyURL(u *url.URL) []byte {
	return []byte("URL:" + u.String())
}

func (s *LevelStore) Exist(u *url.URL) (has bool, err error) {
	return s.DB.Has(keyURL(u), nil)
}
func (s *LevelStore) Get(u *url.URL) (uu *crawler.URL, err error) {
	v, err := s.DB.Get(keyURL(u), nil)
	if err != nil {
		return
	}
	uu = &crawler.URL{}
	err = s.codec.Unmarshal(v, uu)
	return
}
func (s *LevelStore) GetDepth(u *url.URL) (depth int, err error) {
	uu, err := s.Get(u)
	return uu.Depth, err
}
func (s *LevelStore) GetExtra(u *url.URL) (extra interface{}, err error) {
	uu, err := s.Get(u)
	return uu.Extra, err
}

func (s *LevelStore) PutNX(u *crawler.URL) (ok bool, err error) {
	tx, err := s.DB.OpenTransaction()
	if err != nil {
		return
	}
	commit := false
	defer func() {
		if !commit && (err != nil || !ok) {
			tx.Discard() // TODO: handle error
		}
	}()

	key := keyURL(&u.URL)
	has, err := tx.Has(key, nil)
	if err != nil {
		return
	} else if has {
		return false, nil
	}

	v, err := s.codec.Marshal(u)
	if err != nil {
		return
	}
	if err = tx.Put(key, v, nil); err != nil {
		return
	}

	if v, err = tx.Get(keyURLCount, nil); err == nil {
		cnt := util.Btoi64(v) + 1
		if err = tx.Put(keyURLCount, util.I64tob(cnt), nil); err == nil {
			commit = true
			if err = tx.Commit(); err == nil {
				ok = true
			}
		}
	}
	return
}

func (s *LevelStore) Update(u *crawler.URL) (err error) {
	tx, err := s.DB.OpenTransaction()
	if err != nil {
		return
	}
	commit := false
	defer func() {
		if !commit && err != nil {
			tx.Discard() // TODO: handle error
		}
	}()

	key := keyURL(&u.URL)
	v, err := tx.Get(key, nil)
	if err != nil {
		return
	}
	var uu crawler.URL
	if err = s.codec.Unmarshal(v, &uu); err != nil {
		return
	}
	uu.Update(u)
	if v, err = s.codec.Marshal(&uu); err == nil {
		if err = tx.Put(key, v, nil); err == nil {
			commit = true
			err = tx.Commit()
		}
	}
	return
}

func (s *LevelStore) UpdateExtra(u *url.URL, extra interface{}) (err error) {
	tx, err := s.DB.OpenTransaction()
	if err != nil {
		return
	}
	commit := false
	defer func() {
		if !commit && err != nil {
			tx.Discard() // TODO: handle error
		}
	}()

	key := keyURL(u)
	v, err := tx.Get(key, nil)
	if err != nil {
		return
	}
	var uu crawler.URL
	if err = s.codec.Unmarshal(v, &uu); err != nil {
		return
	}
	uu.Extra = extra
	if v, err = s.codec.Marshal(&uu); err == nil {
		if err = tx.Put(key, v, nil); err == nil {
			commit = true
			err = tx.Commit()
		}
	}
	return
}

func (s *LevelStore) Complete(u *url.URL) (err error) {
	tx, err := s.DB.OpenTransaction()
	if err != nil {
		return
	}
	commit := false
	defer func() {
		if !commit && err != nil {
			tx.Discard() // TODO: handle error
		}
	}()

	key := keyURL(u)
	v, err := tx.Get(key, nil)
	if err != nil {
		return
	}
	uu := crawler.URL{}
	if err = s.codec.Unmarshal(v, &uu); err != nil {
		return
	}
	uu.Done = true
	if v, err = s.codec.Marshal(&uu); err != nil {
		return
	}
	if err = tx.Put(key, v, nil); err != nil {
		return
	}
	if v, err = tx.Get(keyFinishCount, nil); err != nil {
		return
	}
	cnt := util.Btoi64(v) + 1
	if err = tx.Put(keyFinishCount, util.I64tob(cnt), nil); err == nil {
		commit = true
		err = tx.Commit()
	}
	return
}

func (s *LevelStore) incCount(k []byte) (err error) {
	tx, err := s.DB.OpenTransaction()
	if err != nil {
		return
	}
	commit := false
	var v []byte
	if v, err = tx.Get(k, nil); err == nil {
		cnt := util.Btoi64(v) + 1
		if err = tx.Put(k, util.I64tob(cnt), nil); err == nil {
			commit = true
			err = tx.Commit()
		}
	}
	if !commit && err != nil {
		tx.Discard() // TODO: handle error
	}
	return
}

func (s *LevelStore) IncVisitCount() (err error) {
	return s.incCount(keyVisitCount)
}
func (s *LevelStore) IncErrorCount() (err error) {
	return s.incCount(keyErrorCount)
}
func (s *LevelStore) IsFinished() (is bool, err error) {
	snap, err := s.DB.GetSnapshot()
	if err != nil {
		return
	}
	defer snap.Release()

	v, err := snap.Get(keyURLCount, nil)
	if err != nil {
		return
	}
	urlcnt := util.Btoi64(v)
	if v, err = snap.Get(keyFinishCount, nil); err == nil {
		if finish := util.Btoi64(v); finish >= urlcnt {
			is = true
		}
	}
	return
}
