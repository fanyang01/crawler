package crawler

import (
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func mustParseURL(ur string) url.URL {
	u, err := url.Parse(ur)
	if err != nil {
		panic(err)
	}
	return *u
}

func mustParseInt(s string) int {
	i, err := strconv.ParseInt(s, 0, 32)
	if err != nil {
		panic(err)
	}
	return int(i)
}

func TestPQ(t *testing.T) {
	pq := newPQueue(100)
	pq.Push(&URL{
		Score: 300,
		Loc:   mustParseURL("/300"),
	})
	pq.Push(&URL{
		Score: 100,
		Loc:   mustParseURL("/100"),
	})
	pq.Push(&URL{
		Score: 200,
		Loc:   mustParseURL("/200"),
	})
	var u *URL
	u = pq.Pop()
	assert.Equal(t, "/300", u.Loc.Path)
	u = pq.Pop()
	assert.Equal(t, "/200", u.Loc.Path)
	u = pq.Pop()
	assert.Equal(t, "/100", u.Loc.Path)
}

func TestWQ(t *testing.T) {
	wq := newWQueue(100)
	now := time.Now()
	wq.Push(&URL{
		nextTime: now.Add(150 * time.Millisecond),
		Loc:      mustParseURL("/150"),
	})
	wq.Push(&URL{
		nextTime: now.Add(100 * time.Millisecond),
		Loc:      mustParseURL("/100"),
	})
	wq.Push(&URL{
		nextTime: now.Add(200 * time.Millisecond),
		Loc:      mustParseURL("/200"),
	})
	time.Sleep(200 * time.Millisecond)
	var u *URL
	var ok bool
	u, ok = wq.Pop()
	assert.True(t, ok)
	assert.Equal(t, "/100", u.Loc.Path)
	u, ok = wq.Pop()
	assert.True(t, ok)
	assert.Equal(t, "/150", u.Loc.Path)
	u, ok = wq.Pop()
	assert.True(t, ok)
	assert.Equal(t, "/200", u.Loc.Path)
}
