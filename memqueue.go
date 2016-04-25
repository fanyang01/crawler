package crawler

import (
	"container/heap"
	"net/url"
	"sync"
	"time"

	"github.com/fanyang01/crawler/queue"
)

// MemQueue represents a bounded blocking wait queue.
type MemQueue struct {
	mu       sync.Mutex
	popCond  *sync.Cond
	pushCond *sync.Cond
	timer    *time.Timer
	chIn     chan *queue.Item
	chOut    chan *url.URL
	closed   bool

	heap queue.Heap
	max  int
}

// NewMemQueue returns a new wait queue that holds at most max items.
func NewMemQueue(max int) *MemQueue {
	q := &MemQueue{
		max: max,
	}
	q.popCond = sync.NewCond(&q.mu)
	q.pushCond = sync.NewCond(&q.mu)
	return q
}

// Channel creates channels which are expected to be used in select statement.
// The returned error channel is always nil.
func (q *MemQueue) Channel() (push chan<- *queue.Item, pop <-chan *url.URL, err <-chan error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.chIn != nil {
		return q.chIn, q.chOut, nil
	}

	err = nil // Memory queue will never get an error.
	q.chIn = make(chan *queue.Item, 32)
	// Small output buffer size means that we pop an item only when it's requested.
	q.chOut = make(chan *url.URL, 1)
	go func() {
		for item := range q.chIn {
			q.Push(item)
		}
	}()
	go func() {
		for {
			if url, _ := q.Pop(); url != nil {
				q.chOut <- url
				continue
			}
			close(q.chOut)
			return
		}
	}()
	return q.chIn, q.chOut, nil
}

// Push will block until there is a room for the item. An error will be
// reported if the queue is closed.
func (q *MemQueue) Push(item *queue.Item) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	for !q.closed && q.heap.Len() >= q.max {
		q.pushCond.Wait()
	}
	if q.closed {
		return queue.ErrPushClosed
	}

	heap.Push(&q.heap, item)
	q.popCond.Signal()
	return nil
}

// Pop will block if heap is empty or none of items should be removed at now.
// It will return nil without error if the queue was closed.
func (q *MemQueue) Pop() (u *url.URL, _ error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	var now time.Time
	var item *queue.Item
	wait := false

WAIT:
	for !q.closed && (q.heap.Len() == 0 || wait) {
		q.popCond.Wait()
		wait = false
	}
	if q.closed {
		return
	}

	item = q.heap.Top()
	now = time.Now()

	if item.Next.Before(now) {
		heap.Pop(&q.heap)
		u = item.URL
		item.Free()
		q.pushCond.Signal()
		return
	}

	if q.timer != nil {
		q.timer.Stop()
	}
	q.timer = time.AfterFunc(item.Next.Sub(now), func() {
		q.mu.Lock()
		q.popCond.Signal()
		q.mu.Unlock()
	})
	wait = true
	goto WAIT
}

func (q *MemQueue) Close() error {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.closed = true
	q.popCond.Broadcast()
	q.pushCond.Broadcast()
	return nil
}
