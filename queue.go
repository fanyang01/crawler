package crawler

import (
	"container/heap"
	"sync"
	"time"
)

// PQ is priority queue, using SchedItem.Next and SchedItem.Score as
// compound priority. It can be operated in two modes: channel mode and
// method mode, but only one mode can be used for a single instance.
type PQ interface {
	// Channel returns a pair of channel for push and pop operations.
	Channel() (push chan<- *SchedItem, pop <-chan *SchedItem)
	// Push inserts an item into queue.
	Push(*SchedItem)
	// Pop removes an item from queue.
	Pop() *SchedItem
	// Close closes queue.
	Close()
}

type baseHeap []*SchedItem

// Less compares compound priority of items.
func (q baseHeap) Less(i, j int) bool {
	if q[i].Next.Before(q[j].Next) {
		return true
	}
	ti := q[i].Next.Round(time.Microsecond)
	tj := q[j].Next.Round(time.Microsecond)
	if ti.After(tj) {
		return false
	}
	// ti = tj
	return q[i].Score > q[j].Score
}
func (q baseHeap) Top() interface{}    { return q[0] }
func (q baseHeap) Len() int            { return len(q) }
func (q baseHeap) Swap(i, j int)       { q[i], q[j] = q[j], q[i] }
func (q *baseHeap) Push(x interface{}) { *q = append(*q, x.(*SchedItem)) }
func (q *baseHeap) Pop() interface{} {
	n := len(*q)
	v := (*q)[n-1]
	*q = (*q)[0 : n-1]
	return v
}

type qHeap struct{ baseHeap }

type pqueue struct {
	heap interface {
		heap.Interface
		Top() interface{}
	}
	maxLen            int
	closed            bool
	chMode            bool
	chIn, chOut       chan *SchedItem
	popCond, pushCond *sync.Cond
	timer             *time.Timer
	quit              chan struct{}
	*sync.Mutex
}

func newPQueue(maxLen int) *pqueue {
	q := &pqueue{
		heap:   &qHeap{},
		maxLen: maxLen,
		Mutex:  new(sync.Mutex),
	}
	q.popCond = sync.NewCond(q.Mutex)
	q.pushCond = sync.NewCond(q.Mutex)
	return q
}

func (q *pqueue) Channel() (push chan<- *SchedItem, pop <-chan *SchedItem) {
	q.Lock()
	defer q.Unlock()

	if q.chIn != nil && q.chOut != nil {
		return q.chIn, q.chOut
	}

	q.chIn = make(chan *SchedItem, 32)
	// Small output buffer size means that we pop an item only when it's requested.
	q.chOut = make(chan *SchedItem, 1)
	go func() {
		for {
			select {
			case item := <-q.chIn:
				q.Push(item)
			case <-q.quit:
				return
			}
		}
	}()
	go func() {
		for {
			if item := q.Pop(); item != nil {
				q.chOut <- item
				continue
			}
			return
		}
	}()
	return q.chIn, q.chOut
}

func (q *pqueue) Push(u *SchedItem) {
	q.Lock()
	defer q.Unlock()
	for !q.closed && q.heap.Len() >= q.maxLen {
		q.pushCond.Wait()
	}
	if q.closed {
		return
	}
	heap.Push(q.heap, u)
	q.popCond.Signal()
}

// Pop will block if heap is empty or none of items should be removed at now.
func (q *pqueue) Pop() *SchedItem {
	q.Lock()
	defer q.Unlock()

	var item *SchedItem
	var now time.Time
	wait := false

WAIT:
	for !q.closed && (q.heap.Len() == 0 || wait) {
		q.popCond.Wait()
		wait = false
	}
	if q.closed {
		return nil
	}

	item = q.heap.Top().(*SchedItem)
	now = time.Now()

	if item.Next.Before(now) {
		item = heap.Pop(q.heap).(*SchedItem)
		q.pushCond.Signal()
		return item
	}

	if q.timer != nil {
		q.timer.Stop()
	}
	q.timer = time.AfterFunc(item.Next.Sub(now), func() {
		q.Lock()
		q.popCond.Signal()
		q.Unlock()
	})
	wait = true
	goto WAIT
}

func (q *pqueue) Close() {
	q.Lock()
	defer q.Unlock()
	q.closed = true
	q.popCond.Broadcast()
	q.pushCond.Broadcast()
}
