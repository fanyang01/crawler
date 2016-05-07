package crawler

import (
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/fanyang01/crawler/queue"
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

func newTestCrawler() *Crawler {
	opt := DefaultOption
	store := NewMemStore()
	ctrl := DefaultController
	cw := &Crawler{
		opt:   opt,
		store: store,
		ctrl:  ctrl,
	}
	return cw
}

func TestQueuePriority(t *testing.T) {
	pq := NewMemQueue(100)
	now := time.Now()
	pq.Push(&queue.Item{
		Score: 300,
		URL:   mustParseURL("/300"),
		Next:  now.Add(50 * time.Millisecond),
	})
	pq.Push(&queue.Item{
		Score: 100,
		URL:   mustParseURL("/100"),
		Next:  now.Add(50 * time.Millisecond),
	})
	pq.Push(&queue.Item{
		Score: 200,
		URL:   mustParseURL("/200"),
		Next:  now.Add(50 * time.Millisecond),
	})
	item, _ := pq.Pop()
	assert.Equal(t, "/300", item.URL.Path)
	item, _ = pq.Pop()
	assert.Equal(t, "/200", item.URL.Path)
	item, _ = pq.Pop()
	assert.Equal(t, "/100", item.URL.Path)
}

func TestQueueTime(t *testing.T) {
	wq := NewMemQueue(100)
	now := time.Now()
	items := []*queue.Item{
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
		item, _ := wq.Pop()
		assert.Equal(t, exp[i], item.URL.Path)
	}
}
