package crawler

import (
	"net/url"
	"sync"
	"time"

	"github.com/fanyang01/bheap"
)

func lessPriority(x, y interface{}) bool {
	// a := *(**URL)(bheap.ValuePtr(x))
	// b := *(**URL)(bheap.ValuePtr(y))
	a, b := x.(*URL), y.(*URL)
	return a.Priority < b.Priority
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

type tqueue struct {
	urlQueue
}

type pqueue struct {
	urlQueue
}

func newPQueue(maxLen int) *pqueue {
	return &pqueue{newURLQueue(lessPriority, maxLen)}
}
func newTQueue(maxLen int) *tqueue {
	return &tqueue{newURLQueue(lessTime, maxLen)}
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

// Pop will block if h is empty
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

func (q *urlQueue) Len() int {
	q.RLock()
	length := q.heap.Len()
	q.RUnlock()
	return length
}

func (pq *pqueue) Pop() (u url.URL) { return pq.urlQueue.Pop().Loc }

func (tq *tqueue) IsAvailable() bool {
	tq.RLock()
	defer tq.RUnlock()
	if v, ok := tq.heap.Top(); ok {
		if !v.(*URL).nextTime.After(time.Now()) {
			return true
		}
	}
	return false
}

func (tq *tqueue) Pop() (*URL, bool) {
	tq.Lock()
	defer tq.Unlock()
	if v, ok := tq.heap.Top(); ok {
		if !v.(*URL).nextTime.After(time.Now()) {
			if v, ok := tq.heap.Pop(); ok {
				return v.(*URL), true
			}
		}
	}
	return nil, false
}

func (tq *tqueue) MultiPop() (s []*URL, any bool) {
	tq.Lock()
	for {
		if v, ok := tq.heap.Top(); ok {
			if !v.(*URL).nextTime.After(time.Now()) {
				if v, ok := tq.heap.Pop(); ok {
					s = append(s, v.(*URL))
					any = true
					continue
				}
			}
		}
		break
	}
	tq.Unlock()
	return
}
