package diskqueue

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/boltdb/bolt"
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

func newTestQueue(t *testing.T, size int) (tmpfile string, q *DiskQueue) {
	f, err := ioutil.TempFile("", "diskqueue")
	if err != nil {
		t.Fatal(err)
	}
	tmpfile = f.Name()
	f.Close()
	db, err := bolt.Open(tmpfile, 0644, nil)
	if err != nil {
		t.Fatal(err)
	}
	q, err = NewDiskQueue(db, DefaultBucket, size, 256)
	if err != nil {
		os.Remove(tmpfile)
		t.Fatal(err)
	}
	return
}

func testTime(t *testing.T, wq *DiskQueue) {
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

func TestNoOverflow(t *testing.T) {
	tmpfile, wq := newTestQueue(t, 100)
	defer os.Remove(tmpfile)
	testTime(t, wq)
}

func TestZeroSize(t *testing.T) {
	tmpfile, wq := newTestQueue(t, 0)
	defer os.Remove(tmpfile)
	testTime(t, wq)
}

func TestOverflow(t *testing.T) {
	tmpfile, wq := newTestQueue(t, 200)
	defer os.Remove(tmpfile)
	now := time.Now()
	for i := 0; i < 2000; i++ {
		wq.Push(&queue.Item{
			// assume all items can be pushed into queue in 1s.
			Next: now.Add(1 * time.Second),
			URL:  mustParseURL(fmt.Sprintf("http://example.com/%d", i)),
		})
	}
	assert := assert.New(t)
	m := map[int]bool{}
	for i := 0; i < 2000; i++ {
		item, _ := wq.Pop()
		if i == 0 {
			assert.True(time.Now().After(now.Add(1 * time.Second)))
		}
		id, _ := strconv.Atoi(strings.TrimPrefix(item.URL.String(), "http://example.com/"))
		assert.False(m[id])
		m[id] = true
	}
}
