package crawler

import (
	"container/heap"
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
	closed   bool

	heap queue.Heap
	max  int
}

// NewMemQueue returns a new wait queue that holds at most max items.
func NewMemQueue(max int) queue.WaitQueue {
	q := &MemQueue{
		max: max,
	}
	q.popCond = sync.NewCond(&q.mu)
	q.pushCond = sync.NewCond(&q.mu)
	return queue.WithChannel(q)
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
func (q *MemQueue) Pop() (item *queue.Item, _ error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	var now time.Time
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
