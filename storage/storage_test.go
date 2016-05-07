package storage

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/fanyang01/crawler"
	"github.com/fanyang01/crawler/storage/boltstore"
	"github.com/fanyang01/crawler/storage/levelstore"

	"github.com/stretchr/testify/assert"
)

func mustParse(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}

func StoreTest(t *testing.T, s crawler.Store) {
	equalTime := func(t1, t2 time.Time) bool {
		return t1.Round(time.Microsecond).Equal(
			t2.Round(time.Microsecond))
	}
	cmp := func(u, uu *crawler.URL) bool {
		return u.URL.String() == uu.URL.String() &&
			u.Depth == uu.Depth &&
			u.Status == uu.Status &&
			equalTime(u.Last, uu.Last) &&
			u.NumVisit == uu.NumVisit &&
			u.NumError == uu.NumError
	}
	tm := time.Now().UTC()
	assert := assert.New(t)
	u, _ := url.Parse("http://localhost:6060")
	uu := &crawler.URL{
		URL:  *u,
		Last: tm,
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
	// assert.Equal(*uu, *uuu)
	assert.True(cmp(uu, uuu))

	uuu.NumVisit++
	uuu.Last = time.Now().UTC()
	assert.NoError(s.Update(uuu))
	uu, err = s.Get(u)
	assert.NoError(err)
	// assert.Equal(*uuu, *uu)
	assert.True(cmp(uu, uuu))

	uu.Status = crawler.URLStatusError
	assert.NoError(s.Update(uuu))
	uuu, err = s.Get(u)
	assert.NoError(err)
	// assert.NotEqual(*uu, *uuu)
	assert.False(cmp(uu, uuu))

	ok, err = s.IsFinished()
	assert.NoError(err)
	assert.False(ok)

	assert.NoError(s.Complete(u))
	ok, err = s.IsFinished()
	assert.NoError(err)
	assert.True(ok)

	u.Path = "/hello"
	uu = &crawler.URL{
		URL: *u,
	}
	ok, err = s.PutNX(uu)
	assert.NoError(err)
	assert.True(ok)
	ok, err = s.IsFinished()
	assert.NoError(err)
	assert.False(ok)

	assert.NoError(s.Complete(u))
	ok, err = s.IsFinished()
	assert.NoError(err)
	assert.True(ok)
}

func TestBolt(t *testing.T) {
	f, err := ioutil.TempFile("", "test_bolt")
	if err != nil {
		t.Fatal(err)
	}
	tmpfile := f.Name()
	f.Close()
	defer os.Remove(tmpfile)

	bs, err := boltstore.New(tmpfile, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	StoreTest(t, bs)
}

func TestLevel(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "test_level")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)

	ls, err := levelstore.New(tmpdir, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	StoreTest(t, ls)
}

func TestMemStore(t *testing.T) {
	ms := crawler.NewMemStore()
	StoreTest(t, ms)
}

func benchPut(b *testing.B, store crawler.Store, name string) {
	// start := time.Now()
	for i := 0; i < b.N; i++ {
		now := time.Now().UTC()
		store.PutNX(&crawler.URL{
			URL:  *mustParse(fmt.Sprintf("http://example.com/foo/bar/%d", i)),
			Last: now,
		})
	}
	// d := time.Now().Sub(start)
	// d /= time.Duration(int64(b.N))
	// b.Log(name, "Put:", d)
}

func BenchmarkMemPut(b *testing.B) {
	ms := crawler.NewMemStore()
	benchPut(b, ms, "MemStore")
}

func BenchmarkBoltPut(b *testing.B) {
	f, err := ioutil.TempFile("", "test_bolt")
	if err != nil {
		b.Fatal(err)
	}
	tmpfile := f.Name()
	f.Close()
	defer os.Remove(tmpfile)

	bs, err := boltstore.New(tmpfile, nil, nil)
	if err != nil {
		b.Fatal(err)
	}
	// b.N = 2000
	benchPut(b, bs, "BoltStore")
}

func BenchmarkLevelPut(b *testing.B) {
	tmpdir, err := ioutil.TempDir("", "test_level")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)

	ls, err := levelstore.New(tmpdir, nil, nil)
	if err != nil {
		b.Fatal(err)
	}
	// b.N = 500
	benchPut(b, ls, "LevelStore")
}

type bench_get_store struct {
	store crawler.Store
	once  sync.Once
}

var (
	testMem, testBolt, testLevel bench_get_store
	tmpbolt                      = "/tmp/bench_get_bolt.db"
	tmplevel                     = "/tmp/bench_get_level.db.d"
	bench_get_size               = 1000
)

func benchGet(b *testing.B, store crawler.Store, name string) {
	// start := time.Now()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		n := rand.Intn(2 * bench_get_size)
		store.Get(mustParse(fmt.Sprintf("http://example.com/foo/bar/%d", n)))
	}
	// d := time.Now().Sub(start) / 2
	// d /= time.Duration(int64(b.N))
	// b.Log(name, "Get:", d)
}

func initGet() {
	init := func(st crawler.Store) {
		for i := 0; i < bench_get_size; i++ {
			now := time.Now().UTC()
			st.PutNX(&crawler.URL{
				URL:  *mustParse(fmt.Sprintf("http://example.com/foo/bar/%d", i)),
				Last: now,
			})
		}
	}
	testMem.once.Do(func() {
		testMem.store = crawler.NewMemStore()
		init(testMem.store)
	})
	testBolt.once.Do(func() {
		os.Remove(tmpbolt)
		testBolt.store, _ = boltstore.New(tmpbolt, nil, nil)
		init(testBolt.store)
	})
	testLevel.once.Do(func() {
		os.RemoveAll(tmplevel)
		testLevel.store, _ = levelstore.New(tmplevel, nil, nil)
		init(testLevel.store)
	})
}

func BenchmarkMemGet(b *testing.B) {
	initGet()
	benchGet(b, testMem.store, "MemStore")
}

func BenchmarkBoltGet(b *testing.B) {
	initGet()
	benchGet(b, testBolt.store, "BoltStore")
}

func BenchmarkLevelGet(b *testing.B) {
	initGet()
	benchGet(b, testLevel.store, "LevelStore")
}
