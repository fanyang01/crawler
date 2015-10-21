package crawler

import (
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func mustParseURL(ur string) *url.URL {
	u, err := url.Parse(ur)
	if err != nil {
		panic(err)
	}
	return u
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
	pq.Push(&SchedItem{
		Score: 300,
		URL:   mustParseURL("/300"),
	})
	pq.Push(&SchedItem{
		Score: 100,
		URL:   mustParseURL("/100"),
	})
	pq.Push(&SchedItem{
		Score: 200,
		URL:   mustParseURL("/200"),
	})
	var u *SchedItem
	u = pq.Pop()
	assert.Equal(t, "/300", u.URL.Path)
	u = pq.Pop()
	assert.Equal(t, "/200", u.URL.Path)
	u = pq.Pop()
	assert.Equal(t, "/100", u.URL.Path)
}

func TestWQ(t *testing.T) {
	wq := newWQueue(100)
	now := time.Now()
	wq.Push(&SchedItem{
		Next: now.Add(150 * time.Millisecond),
		URL:  mustParseURL("/150"),
	})
	wq.Push(&SchedItem{
		Next: now.Add(100 * time.Millisecond),
		URL:  mustParseURL("/100"),
	})
	wq.Push(&SchedItem{
		Next: now.Add(200 * time.Millisecond),
		URL:  mustParseURL("/200"),
	})
	time.Sleep(200 * time.Millisecond)
	var u *SchedItem
	var ok bool
	u, ok = wq.Pop()
	assert.True(t, ok)
	assert.Equal(t, "/100", u.URL.Path)
	u, ok = wq.Pop()
	assert.True(t, ok)
	assert.Equal(t, "/150", u.URL.Path)
	u, ok = wq.Pop()
	assert.True(t, ok)
	assert.Equal(t, "/200", u.URL.Path)
}
