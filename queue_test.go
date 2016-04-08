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

func TestQueuePriority(t *testing.T) {
	cw := newTestCrawler()
	pq := cw.NewMemQueue(100)
	now := time.Now()
	pq.Push(&SchedItem{
		Score: 300,
		URL:   mustParseURL("/300"),
		Next:  now.Add(50 * time.Millisecond),
	})
	pq.Push(&SchedItem{
		Score: 100,
		URL:   mustParseURL("/100"),
		Next:  now.Add(50 * time.Millisecond),
	})
	pq.Push(&SchedItem{
		Score: 200,
		URL:   mustParseURL("/200"),
		Next:  now.Add(50 * time.Millisecond),
	})
	var u *SchedItem
	u = pq.Pop()
	assert.Equal(t, "/300", u.URL.Path)
	u = pq.Pop()
	assert.Equal(t, "/200", u.URL.Path)
	u = pq.Pop()
	assert.Equal(t, "/100", u.URL.Path)
}

func TestQueueTime(t *testing.T) {
	cw := newTestCrawler()
	wq := cw.NewMemQueue(100)
	now := time.Now()
	items := []*SchedItem{
		{
			Next: now.Add(50 * time.Millisecond),
			URL:  mustParseURL("http://a.example.com/50"),
		}, {
			Next: now.Add(75 * time.Millisecond),
			URL:  mustParseURL("http://b.example.com/75"),
		}, {
			Next: now.Add(25 * time.Millisecond),
			URL:  mustParseURL("http://a.example.com/25"),
		}, {
			Next: now.Add(100 * time.Millisecond),
			URL:  mustParseURL("http://b.example.com/100"),
		},
	}
	exp := []string{
		"/25",
		"/50",
		"/75",
		"/100",
	}
	for _, item := range items {
		wq.Push(item)
	}
	for i := 0; i < len(items); i++ {
		u := wq.Pop()
		assert.Equal(t, exp[i], u.URL.Path)
	}
}

type _testIntervalController struct {
	OnceController
}

func (c _testIntervalController) Interval(host string) time.Duration {
	switch host {
	case "a.example.com":
		return 50 * time.Millisecond
	case "b.example.com":
		return 25 * time.Millisecond
	default:
		return 0
	}
}

func TestQueueInterval(t *testing.T) {
	ctrl := _testIntervalController{}
	cw := NewCrawler(nil, nil, ctrl)
	wq := cw.NewMemQueue(100)
	now := time.Now()
	items := []*SchedItem{
		{
			Next: now.Add(25 * time.Millisecond),
			URL:  mustParseURL("http://a.example.com/25"),
		}, {
			Next: now.Add(50 * time.Millisecond),
			URL:  mustParseURL("http://a.example.com/50"),
		}, {
			Next: now.Add(60 * time.Millisecond),
			URL:  mustParseURL("http://b.example.com/60"),
		}, {
			Next: now.Add(100 * time.Millisecond),
			URL:  mustParseURL("http://b.example.com/100"),
		},
	}
	exp := []string{
		"/25",
		"/60",
		"/50",
		"/100",
	}
	for _, item := range items {
		wq.Push(item)
	}
	for i := 0; i < len(items); i++ {
		u := wq.Pop()
		assert.Equal(t, exp[i], u.URL.Path)
	}
}
