package crawler

import (
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"sync"
	"time"

	"github.com/boltdb/bolt"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
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

type PersistableStore interface {
	Store
	// Recover writes all unfinished URLs to w, seperated by '\n'.
	Recover(w io.Writer) (n int, err error)
}

type Encoder interface {
	Marshal(interface{}) ([]byte, error)
	Unmarshal([]byte, interface{}) error
}

type JsonEncoder struct{}

func (_ JsonEncoder) Marshal(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}
func (_ JsonEncoder) Unmarshal(b []byte, v interface{}) error {
	return json.Unmarshal(b, v)
}

type MemStore struct {
	sync.RWMutex
	m map[url.URL]*URL

	URLs     int32
	Finished int32
	Ntimes   int32
	Errors   int32
}

func NewMemStore() *MemStore {
	return &MemStore{
		m: make(map[url.URL]*URL),
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
	DB      *bolt.DB
	filter  *BloomFilter
	encoder Encoder
}

var (
	bkURL          = []byte("URL_BUCKET")
	bkCount        = []byte("CNT_BUCKET")
	keyVisitCount  = []byte("VISIT_COUNT_BUCKET")
	keyURLCount    = []byte("URL_COUNT_BUCKET")
	keyErrorCount  = []byte("ERROR_COUNT_BUCKET")
	keyFinishCount = []byte("FINISH_COUNT_BUCKET")
)

func NewBoltStore(path string, opt *bolt.Options, e Encoder) (bs *BoltStore, err error) {
	if e == nil {
		e = JsonEncoder{}
	}
	bs = &BoltStore{
		filter:  NewBloomFilter(-1, 0.001),
		encoder: e,
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
	// err = s.DB.View(func(tx *bolt.Tx) error {
	// 	b := tx.Bucket(bkURL)
	// 	if b.Get([]byte(u.String())) != nil {
	// 		yes = true
	// 	}
	// 	return nil
	// })
	return s.filter.Exist(u), nil
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
		v, err := s.encoder.Marshal(u)
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
	if err == nil && ok {
		s.filter.Add(&u.Loc)
	}
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
		v, err := s.encoder.Marshal(uu)
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
		v, err := s.encoder.Marshal(uu)
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

func (s *BoltStore) Recover(w io.Writer) (n int, err error) {
	err = s.DB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bkURL)
		c := b.Cursor()
		var u URL
		for k, v := c.First(); k != nil; k, v = c.Next() {
			if err = s.encoder.Unmarshal(v, &u); err != nil {
				return err
			}
			switch u.Status {
			case URLprocessing:
				if _, err = fmt.Fprintln(w, k); err != nil {
					return err
				}
				n++
			}
		}
		return nil
	})
	return
}

type LevelStore struct {
	DB      *leveldb.DB
	encoder Encoder
}

func NewLevelStore(path string, o *opt.Options, e Encoder) (s *LevelStore, err error) {
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
		if err = db.Put(k, i64tob(0), nil); err != nil {
			return
		}
	}
	if e == nil {
		e = JsonEncoder{}
	}
	return &LevelStore{
		DB:      db,
		encoder: e,
	}, nil
}

func keyURL(u *url.URL) []byte {
	return []byte("URL:" + u.String())
}

func (s *LevelStore) Exist(u *url.URL) (has bool, err error) {
	return s.DB.Has(keyURL(u), nil)
}
func (s *LevelStore) Get(u *url.URL) (uu *URL, err error) {
	v, err := s.DB.Get(keyURL(u), nil)
	if err != nil {
		return
	}
	uu = &URL{}
	err = s.encoder.Unmarshal(v, uu)
	return
}
func (s *LevelStore) GetDepth(u *url.URL) (depth int, err error) {
	uu, err := s.Get(u)
	return uu.Depth, err
}

func (s *LevelStore) PutNX(u *URL) (ok bool, err error) {
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

	key := keyURL(&u.Loc)
	has, err := tx.Has(key, nil)
	if err != nil {
		return
	} else if has {
		return false, nil
	}

	v, err := s.encoder.Marshal(u)
	if err != nil {
		return
	}
	if err = tx.Put(key, v, nil); err != nil {
		return
	}

	if v, err = tx.Get(keyURLCount, nil); err == nil {
		cnt := btoi64(v) + 1
		if err = tx.Put(keyURLCount, i64tob(cnt), nil); err == nil {
			commit = true
			if err = tx.Commit(); err == nil {
				ok = true
			}
		}
	}
	return
}

func (s *LevelStore) Update(u *URL) (err error) {
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

	key := keyURL(&u.Loc)
	v, err := tx.Get(key, nil)
	if err != nil {
		return
	}
	var uu URL
	if err = s.encoder.Unmarshal(v, &uu); err != nil {
		return
	}
	uu.update(u)
	if v, err = s.encoder.Marshal(&uu); err == nil {
		if err = tx.Put(key, v, nil); err == nil {
			commit = true
			err = tx.Commit()
		}
	}
	return
}

func (s *LevelStore) UpdateStatus(u *url.URL, status int) (err error) {
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
	uu := URL{}
	if err = s.encoder.Unmarshal(v, &uu); err != nil {
		return
	}
	uu.Status = status
	if v, err = s.encoder.Marshal(&uu); err != nil {
		return
	}
	if err = tx.Put(key, v, nil); err != nil {
		return
	}
	switch status {
	default:
		commit = true
		err = tx.Commit()
		return
	case URLfinished, URLerror:
	}
	if v, err = tx.Get(keyFinishCount, nil); err != nil {
		return
	}
	cnt := btoi64(v) + 1
	if err = tx.Put(keyFinishCount, i64tob(cnt), nil); err == nil {
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
		cnt := btoi64(v) + 1
		if err = tx.Put(k, i64tob(cnt), nil); err == nil {
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
	urlcnt := btoi64(v)
	if v, err = snap.Get(keyFinishCount, nil); err == nil {
		if finish := btoi64(v); finish >= urlcnt {
			is = true
		}
	}
	return
}

type SQLStore struct {
	DB     *sql.DB
	filter *BloomFilter
}

const (
	urlSchema = `
CREATE TABLE IF NOT EXISTS url (
	scheme TEXT,
	host TEXT,
	path TEXT,
	query TEXT,
	depth INT,
	status INT,
	freq NUMERIC,
	last_mod TIMESTAMP,
	last_time TIMESTAMP,
	visit_count INT,
	err_count INT,
	PRIMARY KEY (scheme, host, path, query)
)`
	countSchema = `
CREATE TABLE IF NOT EXISTS count (
	url_count INT,
	finish_count INT,
	error_count INT,
	visit_count INT
)
`
)

func NewSQLStore(driver, uri string) (s *SQLStore, err error) {
	db, err := sql.Open(driver, uri)
	if err != nil {
		return
	}
	tx, err := db.Begin()
	if err != nil {
		return
	}
	s = &SQLStore{
		DB:     db,
		filter: NewBloomFilter(-1, 0.0001),
	}
	defer func() {
		if err != nil {
			tx.Rollback() // TODO
		} else {
			err = tx.Commit()
		}
	}()

	if _, err = tx.Exec(urlSchema); err != nil {
		return
	}
	if _, err = tx.Exec(countSchema); err != nil {
		return
	}
	var cnt int
	if err = tx.QueryRow(
		`SELECT count(*) FROM count`,
	).Scan(&cnt); err != nil {
		return
	} else if cnt == 0 {
		_, err = tx.Exec(
			`INSERT INTO count(url_count, finish_count, error_count, visit_count)
			 VALUES (0, 0, 0, 0)`,
		)
	}
	return
}

func (s *SQLStore) Exist(u *url.URL) (bool, error) {
	return s.filter.Exist(u), nil
}
func (s *SQLStore) Get(u *url.URL) (uu *URL, err error) {
	uu = &URL{}
	err = s.DB.QueryRow(
		`SELECT scheme, host, path, query, depth, status, freq, last_mod, last_time, visit_count, err_count
    	FROM url
    	WHERE scheme = $1 AND host = $2 AND path = $3 AND query = $4`,
		u.Scheme, u.Host, u.Path, u.RawQuery,
	).Scan(
		&uu.Loc.Scheme,
		&uu.Loc.Host,
		&uu.Loc.Path,
		&uu.Loc.RawQuery,
		&uu.Depth,
		&uu.Status,
		&uu.Freq,
		&uu.LastMod,
		&uu.LastTime,
		&uu.VisitCount,
		&uu.ErrCount,
	)
	return
}

func (s *SQLStore) GetDepth(u *url.URL) (depth int, err error) {
	err = s.DB.QueryRow(
		`SELECT depth FROM url
    	WHERE scheme = $1 AND host = $2 AND path = $3 AND query = $4`,
		u.Scheme, u.Host, u.Path, u.RawQuery,
	).Scan(&depth)
	return
}
func (s *SQLStore) PutNX(u *URL) (ok bool, err error) {
	tx, err := s.DB.Begin()
	if err != nil {
		return
	}

	put := false
	defer func() {
		if err != nil {
			tx.Rollback() // TODO: handle error
		} else {
			if err = tx.Commit(); err == nil && put {
				s.filter.Add(&u.Loc)
				ok = true
			}
		}
	}()

	var cnt int
	if err = tx.QueryRow(`
		SELECT count(*) FROM url
    	WHERE scheme = $1 AND host = $2 AND path = $3 AND query = $4`,
		u.Loc.Scheme, u.Loc.Host, u.Loc.Path, u.Loc.RawQuery,
	).Scan(&cnt); err != nil {
		return
	} else if cnt > 0 {
		return
	}

	if _, err = tx.Exec(`
	INSERT INTO url(scheme, host, path, query, depth, status, freq, last_mod, last_time, visit_count, err_count)
	 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		u.Loc.Scheme,
		u.Loc.Host,
		u.Loc.Path,
		u.Loc.RawQuery,
		u.Depth,
		u.Status,
		u.Freq,
		u.LastMod,
		u.LastTime,
		u.VisitCount,
		u.ErrCount,
	); err == nil {
		put = true
		_, err = tx.Exec(
			`UPDATE count SET url_count = url_count + 1`,
		)
	}
	return
}
func (s *SQLStore) Update(u *URL) (err error) {
	_, err = s.DB.Exec(`
	UPDATE url SET err_count = $1, visit_count = $2, last_time = $3, last_mod = $4
	WHERE scheme = $5 AND host = $6 AND path = $7 AND query = $8`,
		u.ErrCount,
		u.VisitCount,
		u.LastTime,
		u.LastMod,

		u.Loc.Scheme,
		u.Loc.Host,
		u.Loc.Path,
		u.Loc.RawQuery,
	)
	return
}
func (s *SQLStore) UpdateStatus(u *url.URL, status int) (err error) {
	tx, err := s.DB.Begin()
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			tx.Rollback() // TODO: handle error
		} else {
			err = tx.Commit()
		}
	}()

	if _, err = tx.Exec(`
	UPDATE url SET status = $1
	WHERE scheme = $2 AND host = $3 AND path = $4 AND query = $5`,
		status,

		u.Scheme,
		u.Host,
		u.Path,
		u.RawQuery,
	); err != nil {
		return
	}
	switch status {
	case URLfinished, URLerror:
		_, err = tx.Exec(
			`UPDATE count SET finish_count = finish_count + 1`,
		)
	}
	return
}

func (s *SQLStore) IncVisitCount() (err error) {
	_, err = s.DB.Exec(
		`UPDATE count SET visit_count = visit_count + 1`,
	)
	return
}
func (s *SQLStore) IncErrorCount() (err error) {
	_, err = s.DB.Exec(
		`UPDATE count SET error_count = error_count + 1`,
	)
	return
}
func (s *SQLStore) IsFinished() (is bool, err error) {
	var rest int
	if err = s.DB.QueryRow(
		`SELECT url_count - finish_count FROM count`,
	).Scan(&rest); err != nil {
		return
	}
	if rest <= 0 {
		is = true
	}
	return
}
