// Package queue defines the wait queue interface.
package queue

import (
	"net/url"
	"sync"
	"time"
)

// Item is the item in wait queue.
type Item struct {
	URL   *url.URL
	Next  time.Time // next time to crawl
	Score int       // optional for implementation
}

var freelist = &sync.Pool{
	New: func() interface{} { return new(Item) },
}

// NewItem allocates an Item object.
func NewItem() *Item {
	return freelist.Get().(*Item)
}

// Free deallocates an Item object.
func (item *Item) Free() {
	item.URL = nil
	item.Next = time.Time{}
	item.Score = 0
	freelist.Put(item)
}

// WaitQueue should be a priority queue, using Item.Next and optional
// Item.Score as priority. It can be operated in two modes: channel mode
// and method mode, but only one mode will be used for a single instance.
type WaitQueue interface {
	// Channel returns a pair of channel for push and pop operations.
	Channel() (push chan<- *Item, pop <-chan *url.URL)
	// Push adds an item to queue.
	Push(*Item)
	// Pop removes an item from queue.
	Pop() *url.URL
	// Close closes queue.
	Close()
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
