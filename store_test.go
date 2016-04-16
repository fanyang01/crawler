package crawler

import (
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func testStore(t *testing.T, s Store) {
	tm := time.Now().UTC()
	assert := assert.New(t)
	u, _ := url.Parse("http://localhost:6060")
	uu := &URL{
		Loc:      *u,
		LastMod:  tm,
		LastTime: tm,
	}
	ok, err := s.PutNX(uu)
	assert.NoError(err)
	assert.True(ok)

	ok, err = s.PutNX(uu)
	assert.NoError(err)
	assert.False(ok)

	ok, err = s.Exist(u)
	assert.NoError(err)
	assert.True(ok)

	uuu, err := s.Get(u)
	assert.NoError(err)
	assert.Equal(*uu, *uuu)

	uuu.VisitCount++
	uuu.LastTime = time.Now().UTC()
	assert.NoError(s.Update(uuu))
	uu, err = s.Get(u)
	assert.NoError(err)
	assert.Equal(*uuu, *uu)

	uu.Status = URLerror
	assert.NoError(s.Update(uuu))
	uuu, err = s.Get(u)
	assert.NoError(err)
	assert.NotEqual(*uu, *uuu)

	ok, err = s.IsFinished()
	assert.NoError(err)
	assert.False(ok)

	assert.NoError(s.UpdateStatus(u, URLfinished))
	ok, err = s.IsFinished()
	assert.NoError(err)
	assert.True(ok)

	u.Path = "/hello"
	uu = &URL{
		Loc: *u,
	}
	ok, err = s.PutNX(uu)
	assert.NoError(err)
	assert.True(ok)
	ok, err = s.IsFinished()
	assert.NoError(err)
	assert.False(ok)

	assert.NoError(s.UpdateStatus(u, URLerror))
	ok, err = s.IsFinished()
	assert.NoError(err)
	assert.True(ok)
}

func TestBolt(t *testing.T) {
	tmpfile := "/tmp/bolt.test.db"
	bs, err := NewBoltStore(tmpfile, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile)
	testStore(t, bs)
}

func TestLevel(t *testing.T) {
	tmpdir := "/tmp/leveldb.test.d"
	ls, err := NewLevelStore(tmpdir, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)
	testStore(t, ls)
}

func TestMemStore(t *testing.T) {
	ms := NewMemStore()
	testStore(t, ms)
}

func benchPut(b *testing.B, store Store, name string) {
	parse := func(s string) *url.URL {
		u, err := url.Parse(s)
		if err != nil {
			panic(err)
		}
		return u
	}
	start := time.Now()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		now := time.Now().UTC()
		store.PutNX(&URL{
			Loc:      *parse(fmt.Sprintf("http://example.com/foo/bar/%d", i)),
			LastTime: now,
			LastMod:  now,
		})
	}
	d := time.Now().Sub(start)
	d /= time.Duration(int64(b.N))
	b.Log(name, "Put:", d)
}

func BenchmarkMemPut(b *testing.B) {
	ms := NewMemStore()
	benchPut(b, ms, "MemStore")
}

func BenchmarkBoltPut(b *testing.B) {
	tmpfile := "/tmp/bolt.test.db"
	bs, err := NewBoltStore(tmpfile, nil, nil)
	if err != nil {
		b.Fatal(err)
	}
	defer os.Remove(tmpfile)
	b.N = 2000
	benchPut(b, bs, "BoltStore")
}

func BenchmarkLevelPut(b *testing.B) {
	tmpdir := "/tmp/leveldb.test.d"
	ls, err := NewLevelStore(tmpdir, nil, nil)
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)
	b.N = 500
	benchPut(b, ls, "LevelStore")
}

func benchGet(b *testing.B, store Store, name string) {
	parse := func(s string) *url.URL {
		u, err := url.Parse(s)
		if err != nil {
			panic(err)
		}
		return u
	}
	for i := 0; i < b.N; i++ {
		now := time.Now().UTC()
		store.PutNX(&URL{
			Loc:      *parse(fmt.Sprintf("http://example.com/foo/bar/%d", i)),
			LastTime: now,
			LastMod:  now,
		})
	}
	b.ResetTimer()
	start := time.Now()
	for i := 0; i < 2*b.N; i++ {
		store.Get(parse(fmt.Sprintf("http://example.com/foo/bar/%d", i)))
	}
	d := time.Now().Sub(start) / 2
	d /= time.Duration(int64(b.N))
	b.Log(name, "Get:", d)
}

func BenchmarkMemGet(b *testing.B) {
	ms := NewMemStore()
	benchGet(b, ms, "MemStore")
}

func BenchmarkBoltGet(b *testing.B) {
	tmpfile := "/tmp/bolt.test.db"
	bs, err := NewBoltStore(tmpfile, nil, nil)
	if err != nil {
		b.Fatal(err)
	}
	defer os.Remove(tmpfile)
	b.N = 100
	benchGet(b, bs, "BoltStore")
}

func BenchmarkLevelGet(b *testing.B) {
	tmpdir := "/tmp/leveldb.test.d"
	ls, err := NewLevelStore(tmpdir, nil, nil)
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)
	b.N = 100
	benchGet(b, ls, "LevelStore")
}
