package crawler

import (
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func testStore(t *testing.T, s Store) {
	assert := assert.New(t)
	u, _ := url.Parse("http://localhost:6060")
	uu := &URL{
		Loc:      *u,
		LastMod:  time.Now(),
		LastTime: time.Now(),
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
	uuu.LastTime = time.Now()
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
	assert := assert.New(t)
	bs, err := NewBoltStore(tmpfile, nil)
	assert.NoError(err)
	defer os.Remove(tmpfile)
	testStore(t, bs)
}

func TestMemStore(t *testing.T) {
	ms := newMemStore()
	testStore(t, ms)
}
