package crawler

import (
	"container/heap"
	"sync"
	"time"
)

// Priority queue, using URL.Score as priority
type PQ interface {
	Push(*URL)
	Pop() *URL
}

// Waiting queue, using URL.nextTime as priority
type WQ interface {
	Push(*URL)
	// Check if any 'nextTime' is before/at now.
	IsAvailable() bool
	// Pop a url whose 'nextTime' is before/at now. No blocking.
	Pop() (*URL, bool)
	// Pop all urls whose 'nextTime' is before/at now. No blocking.
	MultiPop() (urls []*URL, any bool)
}

type baseHeap []*URL

func (q baseHeap) Len() int            { return len(q) }
func (q baseHeap) Swap(i, j int)       { q[i], q[j] = q[j], q[i] }
func (q baseHeap) Top() interface{}    { return q[0] }
func (q *baseHeap) Push(x interface{}) { *q = append(*q, x.(*URL)) }
func (q *baseHeap) Pop() interface{} {
	n := len(*q)
	v := (*q)[n-1]
	*q = (*q)[0 : n-1]
	return v
}

type wHeap struct{ baseHeap }

func (h wHeap) Less(i, j int) bool {
	return h.baseHeap[i].nextTime.Before(h.baseHeap[j].nextTime)
}

type pHeap struct{ baseHeap }

func (h pHeap) Less(i, j int) bool {
	return h.baseHeap[i].Score > h.baseHeap[j].Score
}

type urlQueue struct {
	heap interface {
		heap.Interface
		Top() interface{}
	}
	maxLen int
	pop    *sync.Cond
	push   *sync.Cond
	*sync.RWMutex
}

func newURLQueue(h interface {
	heap.Interface
	Top() interface{}
}, maxLen int) urlQueue {
	var q urlQueue
	q.maxLen = maxLen
	q.RWMutex = new(sync.RWMutex)
	q.heap = h
	q.pop = sync.NewCond(q.RWMutex)
	q.push = sync.NewCond(q.RWMutex)
	return q
}

func (q *urlQueue) Push(u *URL) {
	q.Lock()
	if q.heap.Len() >= q.maxLen {
		q.push.Wait()
	}
	heap.Push(q.heap, u)
	q.pop.Signal()
	q.Unlock()
}

// Pop will block if heap is empty
func (q *urlQueue) Pop() (u *URL) {
	q.Lock()
	for q.heap.Len() == 0 {
		q.pop.Wait()
	}
	defer q.Unlock()
	i := heap.Pop(q.heap)
	q.push.Signal()
	return i.(*URL)
}

type wqueue struct {
	urlQueue
}
type pqueue struct {
	urlQueue
}

func newPQueue(maxLen int) *pqueue {
	return &pqueue{newURLQueue(&pHeap{}, maxLen)}
}
func newTQueue(maxLen int) *wqueue {
	return &wqueue{newURLQueue(&wHeap{}, maxLen)}
}

func (wq *wqueue) IsAvailable() bool {
	wq.RLock()
	defer wq.RUnlock()
	if wq.heap.Len() == 0 {
		return false
	}
	v := wq.heap.Top()
	if !v.(*URL).nextTime.After(time.Now()) {
		return true
	}
	return false
}

func (wq *wqueue) Pop() (*URL, bool) {
	wq.Lock()
	defer wq.Unlock()
	if wq.heap.Len() == 0 {
		return nil, false
	}
	v := wq.heap.Top()
	if !v.(*URL).nextTime.After(time.Now()) {
		v := heap.Pop(wq.heap)
		return v.(*URL), true
	}
	return nil, false
}

func (wq *wqueue) MultiPop() (s []*URL, any bool) {
	wq.Lock()
	for {
		if wq.heap.Len() == 0 {
			break
		}
		v := wq.heap.Top()
		if !v.(*URL).nextTime.After(time.Now()) {
			v := heap.Pop(wq.heap)
			s = append(s, v.(*URL))
			any = true
			continue
		}
		break
	}
	wq.Unlock()
	return
}
