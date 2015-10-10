package crawler

import (
	"sync"
	"time"

	"github.com/fanyang01/bheap"
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

func lessScore(x, y interface{}) bool {
	// a := (*URL)(bheap.ValuePtr(x))
	// b := (*URL)(bheap.ValuePtr(y))
	a, b := x.(*URL), y.(*URL)
	return a.Score < b.Score
}

func lessTime(x, y interface{}) bool {
	a, b := x.(*URL), y.(*URL)
	return a.nextTime.After(b.nextTime)
}

type urlQueue struct {
	maxLen int
	heap   *bheap.Heap
	pop    *sync.Cond
	push   *sync.Cond
	*sync.RWMutex
}

func newURLQueue(f bheap.LessFunc, maxLen int) urlQueue {
	var q urlQueue
	q.maxLen = maxLen
	q.RWMutex = new(sync.RWMutex)
	q.heap = bheap.New(f)
	q.pop = sync.NewCond(q.RWMutex)
	q.push = sync.NewCond(q.RWMutex)
	return q
}

func (q *urlQueue) Push(u *URL) {
	q.Lock()
	if q.heap.Len() >= q.maxLen {
		q.push.Wait()
	}
	q.heap.Push(u)
	q.pop.Signal()
	q.Unlock()
}

// Pop will block if heap is empty
func (q *urlQueue) Pop() (u *URL) {
	q.Lock()
	for q.heap.IsEmpty() {
		q.pop.Wait()
	}
	defer q.Unlock()
	// In this usage, it's impossible for Pop to return nil and false
	i, _ := q.heap.Pop()
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
	return &pqueue{newURLQueue(lessScore, maxLen)}
}
func newTQueue(maxLen int) *wqueue {
	return &wqueue{newURLQueue(lessTime, maxLen)}
}

func (wq *wqueue) IsAvailable() bool {
	wq.RLock()
	defer wq.RUnlock()
	if v, ok := wq.heap.Top(); ok {
		if !v.(*URL).nextTime.After(time.Now()) {
			return true
		}
	}
	return false
}

func (wq *wqueue) Pop() (*URL, bool) {
	wq.Lock()
	defer wq.Unlock()
	if v, ok := wq.heap.Top(); ok {
		if !v.(*URL).nextTime.After(time.Now()) {
			if v, ok := wq.heap.Pop(); ok {
				return v.(*URL), true
			}
		}
	}
	return nil, false
}

func (wq *wqueue) MultiPop() (s []*URL, any bool) {
	wq.Lock()
	for {
		if v, ok := wq.heap.Top(); ok {
			if !v.(*URL).nextTime.After(time.Now()) {
				if v, ok := wq.heap.Pop(); ok {
					s = append(s, v.(*URL))
					any = true
					continue
				}
			}
		}
		break
	}
	wq.Unlock()
	return
}
