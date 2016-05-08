// Package queue defines the wait queue interface.
package queue

import (
	"errors"
	"net/url"
	"sync"
	"time"

	"golang.org/x/net/context"
)

// ErrPushClosed is a general queue error.
var ErrPushClosed = errors.New("queue: can not push on closed queue")

// Item is the item in wait queue.
type Item struct {
	URL  *url.URL
	Next time.Time // next time to crawl

	// optional for implementation
	Score      int             `json:",omitempty"`
	RetryMax   int             `json:",omitempty"`
	RetryDelay time.Duration   `json:",omitempty"`
	Ctx        context.Context `json:"-"`
}

var freelist = &sync.Pool{
	New: func() interface{} { return new(Item) },
}

// NewItem allocates an Item object.
func NewItem() *Item {
	item := freelist.Get().(*Item)
	item.Ctx = context.TODO()
	return item
}

// Free deallocates an Item object.
func (item *Item) Free() {
	item.URL = nil
	item.Next = time.Time{}
	item.Score = 0
	item.Ctx = nil
	freelist.Put(item)
}

// WaitQueue should be a priority queue, using Item.Next and optional
// Item.Score as priority. It can be operated in two modes: channel mode
// and method mode, but only one mode will be used for a single instance.
type WaitQueue interface {
	// Channel returns three channels expected to be used in select
	// statement. It's implementation's responsibility to close the in
	// channel and error channel when Close method is called. Callers
	// should be responsible for closing the out channel.
	// Channel should be goroutine-safe.
	Channel() (in chan<- *Item, out <-chan *Item, err <-chan error)
	// Push adds an item to queue.
	Push(*Item) error
	// Pop removes an item from queue.
	Pop() (*Item, error)
	// Close closes queue. Close will be called even an error has been
	// returned in above operations.
	Close() error
}

// Interface is a helper interface. WithChannel can generate a Channel
// method for implementations.
type Interface interface {
	Push(*Item) error
	Pop() (*Item, error)
	Close() error
}

type channel struct {
	Interface
	quit chan struct{}
	mu   sync.Mutex
	in   chan *Item
	out  chan *Item
	err  chan error
}

// Channel implements WaitQueue.
func (q *channel) Channel() (in chan<- *Item, out <-chan *Item, err <-chan error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.in != nil {
		return q.in, q.out, q.err
	}

	q.in = make(chan *Item, 1024)
	q.err = make(chan error, 1)
	// Small output buffer size means that we pop an item only when it's requested.
	q.out = make(chan *Item, 1)
	go func() {
		for item := range q.in {
			if err := q.Push(item); err != nil {
				q.sendErr(err)
				return
			}
		}
	}()
	go func() {
		for {
			item, err := q.Pop()
			if err != nil {
				q.sendErr(err)
				return
			}
			if item != nil {
				q.out <- item
				continue
			}
			return
		}
	}()
	return q.in, q.out, nil
}

func (q *channel) sendErr(err error) {
	select {
	case q.err <- err:
	case <-q.quit:
	}
}

// Channel implements WaitQueue.
func (q *channel) Close() error {
	close(q.quit)
	return q.Interface.Close()
}

// WithChannel provides a wrapper for those who don't implement a Channel
// method.
func WithChannel(q Interface) WaitQueue {
	return &channel{
		Interface: q,
		quit:      make(chan struct{}),
	}
}

// Heap implements heap.Interface.
type Heap struct {
	S []*Item
}

// Less compares compound priority of items.
func (q *Heap) Less(i, j int) bool {
	if q.S[i].Next.Before(q.S[j].Next) {
		return true
	}
	ti := q.S[i].Next.Round(time.Microsecond)
	tj := q.S[j].Next.Round(time.Microsecond)
	if ti.After(tj) {
		return false
	}
	// ti = tj
	return q.S[i].Score > q.S[j].Score
}
func (q *Heap) Top() *Item         { return q.S[0] }
func (q *Heap) Len() int           { return len(q.S) }
func (q *Heap) Swap(i, j int)      { q.S[i], q.S[j] = q.S[j], q.S[i] }
func (q *Heap) Push(x interface{}) { q.S = append(q.S, x.(*Item)) }
func (q *Heap) Pop() interface{} {
	n := len(q.S)
	v := q.S[n-1]
	q.S = q.S[0 : n-1]
	return v
}
