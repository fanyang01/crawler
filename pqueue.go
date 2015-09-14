package crawler

import (
	"sync"

	"github.com/fanyang01/bheap"
)

func less(x, y interface{}) bool {
	a := *(**URL)(bheap.ValuePtr(x))
	b := *(**URL)(bheap.ValuePtr(y))
	return a.Priority < b.Priority
}

type urlHeap struct {
	*bheap.Heap
	*sync.RWMutex
	*sync.Cond
}

func newURLQueue(length int) *urlHeap {
	h := new(urlHeap)
	h.RWMutex = new(sync.RWMutex)
	h.Heap = bheap.New(less)
	h.Cond = sync.NewCond(h.RWMutex)
	return h
}

func (h *urlHeap) Push(u *URL) {
	h.Lock()
	h.Heap.Push(u)
	h.Unlock()
	h.Signal()
}

// Pop will block if h is empty
func (h *urlHeap) Pop() (u *URL) {
	h.Lock()
	for h.Heap.IsEmpty() {
		h.Wait()
	}
	defer h.Unlock()
	i := h.Heap.Pop()
	if i == nil {
		return
	}
	return i.(*URL)
}
