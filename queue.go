package crawler

import (
	"container/heap"
	"net/url"
	"sync"
	"time"

	"github.com/fanyang01/crawler/queue"
)

type baseHeap struct {
	S []*queue.Item
}

// Less compares compound priority of items.
func (q *baseHeap) Less(i, j int) bool {
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
func (q *baseHeap) Top() *queue.Item   { return q.S[0] }
func (q *baseHeap) Len() int           { return len(q.S) }
func (q *baseHeap) Swap(i, j int)      { q.S[i], q.S[j] = q.S[j], q.S[i] }
func (q *baseHeap) Push(x interface{}) { q.S = append(q.S, x.(*queue.Item)) }
func (q *baseHeap) Pop() interface{} {
	n := len(q.S)
	v := q.S[n-1]
	q.S = q.S[0 : n-1]
	return v
}

type MemQueue struct {
	mu       sync.Mutex
	popCond  *sync.Cond
	pushCond *sync.Cond
	timer    *time.Timer

	chIn   chan *queue.Item
	chOut  chan *url.URL
	closed bool

	heap baseHeap
	max  int
}

func NewMemQueue(max int) *MemQueue {
	q := &MemQueue{
		max: max,
	}
	q.popCond = sync.NewCond(&q.mu)
	q.pushCond = sync.NewCond(&q.mu)
	return q
}

func (q *MemQueue) Channel() (push chan<- *queue.Item, pop <-chan *url.URL) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.chIn != nil && q.chOut != nil {
		return q.chIn, q.chOut
	}

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
			if url := q.Pop(); url != nil {
				q.chOut <- url
				continue
			}
			// close(q.chOut)
			return
		}
	}()
	return q.chIn, q.chOut
}

func (q *MemQueue) Push(item *queue.Item) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for !q.closed && q.heap.Len() >= q.max {
		q.pushCond.Wait()
	}
	if q.closed {
		return
	}

	heap.Push(&q.heap, item)
	q.popCond.Signal()
}

// Pop will block if heap is empty or none of items should be removed at now.
func (q *MemQueue) Pop() *url.URL {
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
		return nil
	}

	item = q.heap.Top()
	now = time.Now()

	if item.Next.Before(now) {
		heap.Pop(&q.heap)
		q.pushCond.Signal()
		return item.URL
	}

	if q.timer != nil {
		q.timer.Stop()
	}
	q.timer = time.AfterFunc(item.Next.Sub(now), func() {
		q.mu.Lock()
		q.timer = nil
		q.popCond.Signal()
		q.mu.Unlock()
	})
	wait = true
	goto WAIT
}

func (q *MemQueue) Close() {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.closed = true
	q.popCond.Broadcast()
	q.pushCond.Broadcast()
}
