package crawler

import (
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
	heap *bheap.Heap
	*sync.RWMutex
	*sync.Cond
}

type tqueue struct {
	urlQueue
}

type pqueue struct {
	urlQueue
}

func newPQueue() *pqueue {
	return &pqueue{newURLQueue(lessPriority)}
}
func newTQueue() *tqueue {
	return &tqueue{newURLQueue(lessTime)}
}

func newURLQueue(f bheap.LessFunc) urlQueue {
	var q urlQueue
	q.RWMutex = new(sync.RWMutex)
	q.heap = bheap.New(f)
	q.Cond = sync.NewCond(q.RWMutex)
	return q
}

func (q *urlQueue) Push(u *URL) {
	q.Lock()
	q.heap.Push(u)
	q.Signal()
	q.Unlock()
}

// Pop will block if h is empty
func (q *urlQueue) Pop() (u *URL) {
	q.Lock()
	for q.heap.IsEmpty() {
		q.Wait()
	}
	defer q.Unlock()
	// In this usage, it's impossible for Pop to return nil and false
	i, _ := q.heap.Pop()
	return i.(*URL)
}

func (q *urlQueue) IsEmpty() bool {
	q.RLock()
	defer q.RUnlock()
	return q.heap.IsEmpty()
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
	defer tq.Unlock()
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
	return
}
