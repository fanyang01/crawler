package crawler

import (
	"container/heap"
	"sync"
	"time"
)

// PQ is priority queue, using SchedItem.Score as priority.
type PQ interface {
	// Push adds a new url to queue. Blocking or not.
	Push(*SchedItem)
	// Pop removes the element of highest priority. Blocking.
	Pop() *SchedItem
	// Close closes queue, wake up all sleeping push/pop.
	Close()
}

// WQ is waiting queue, using SchedItem.Next as priority.
type WQ interface {
	Push(*SchedItem)
	// Check if any 'Next' is before/at now.
	IsAvailable() bool
	// Pop a url whose 'Next' is before/at now. No blocking.
	Pop() (*SchedItem, bool)
	// Pop all urls whose 'Next' is before/at now. No blocking.
	MultiPop() (items []*SchedItem, any bool)
	// Close closes queue, wake up all sleeping push.
	Close()
	IsClosed() bool
}

type baseHeap []*SchedItem

func (q baseHeap) Len() int            { return len(q) }
func (q baseHeap) Swap(i, j int)       { q[i], q[j] = q[j], q[i] }
func (q baseHeap) Top() interface{}    { return q[0] }
func (q *baseHeap) Push(x interface{}) { *q = append(*q, x.(*SchedItem)) }
func (q *baseHeap) Pop() interface{} {
	n := len(*q)
	v := (*q)[n-1]
	*q = (*q)[0 : n-1]
	return v
}

type wHeap struct{ baseHeap }

func (h wHeap) Less(i, j int) bool {
	return h.baseHeap[i].Next.Before(h.baseHeap[j].Next)
}

type pHeap struct{ baseHeap }

func (h pHeap) Less(i, j int) bool {
	return h.baseHeap[i].Score > h.baseHeap[j].Score
}

type pqueue struct {
	heap   heap.Interface
	maxLen int
	pop    *sync.Cond
	push   *sync.Cond
	closed bool
	*sync.RWMutex
}

func newPQueue(maxLen int) *pqueue {
	q := &pqueue{
		heap:    &pHeap{},
		maxLen:  maxLen,
		RWMutex: new(sync.RWMutex),
	}
	q.pop = sync.NewCond(q.RWMutex)
	q.push = sync.NewCond(q.RWMutex)
	return q
}

func (q *pqueue) Push(u *SchedItem) {
	q.Lock()
	defer q.Unlock()
	for !q.closed && q.heap.Len() >= q.maxLen {
		q.push.Wait()
	}
	if q.closed {
		return
	}
	heap.Push(q.heap, u)
	q.pop.Signal()
}

// Pop will block if heap is empty.
func (q *pqueue) Pop() (u *SchedItem) {
	q.Lock()
	defer q.Unlock()
	for !q.closed && q.heap.Len() == 0 {
		q.pop.Wait()
	}
	if q.closed {
		return nil
	}
	i := heap.Pop(q.heap)
	q.push.Signal()
	return i.(*SchedItem)
}

func (q *pqueue) Close() {
	q.Lock()
	q.closed = true
	q.pop.Broadcast()
	q.push.Broadcast()
	q.Unlock()
}

type wqueue struct {
	heap interface {
		heap.Interface
		Top() interface{}
	}
	maxLen int
	closed bool
	push   *sync.Cond
	*sync.RWMutex
}

func newWQueue(maxLen int) *wqueue {
	q := &wqueue{
		heap:    &wHeap{},
		maxLen:  maxLen,
		RWMutex: new(sync.RWMutex),
	}
	q.push = sync.NewCond(q.RWMutex)
	return q
}

func (q *wqueue) Push(u *SchedItem) {
	q.Lock()
	defer q.Unlock()
	for !q.closed && q.heap.Len() >= q.maxLen {
		q.push.Wait()
	}
	if q.closed {
		return
	}
	heap.Push(q.heap, u)
}

func (wq *wqueue) Pop() (*SchedItem, bool) {
	wq.Lock()
	defer wq.Unlock()
	if wq.closed || wq.heap.Len() == 0 {
		return nil, false
	}
	v := wq.heap.Top()
	if !v.(*SchedItem).Next.After(time.Now()) {
		v := heap.Pop(wq.heap)
		wq.push.Signal()
		return v.(*SchedItem), true
	}
	return nil, false
}

func (wq *wqueue) Close() {
	wq.Lock()
	wq.closed = true
	wq.Unlock()
}

func (wq *wqueue) IsClosed() bool {
	wq.RLock()
	defer wq.RUnlock()
	return wq.closed
}

func (wq *wqueue) IsAvailable() bool {
	wq.RLock()
	defer wq.RUnlock()
	if wq.heap.Len() == 0 {
		return false
	}
	v := wq.heap.Top()
	if !v.(*SchedItem).Next.After(time.Now()) {
		return true
	}
	return false
}

func (wq *wqueue) MultiPop() (s []*SchedItem, any bool) {
	wq.Lock()
	defer wq.Unlock()
	for {
		if wq.closed || wq.heap.Len() == 0 {
			break
		}
		v := wq.heap.Top()
		if !v.(*SchedItem).Next.After(time.Now()) {
			v := heap.Pop(wq.heap)
			wq.push.Signal()
			s = append(s, v.(*SchedItem))
			any = true
			continue
		}
		break
	}
	return
}
