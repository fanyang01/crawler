package crawler

import (
	"container/heap"
	"net/url"
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

type SiteInfo struct {
	Interval time.Duration
}

type SchedItem struct {
	URL   *url.URL
	Next  time.Time
	Score int
	Site  *SiteInfo
}

func (si SchedItem) interval() time.Duration {
	if si.Site != nil {
		return si.Site.Interval
	}
	return 0
}

type primaryItem struct {
	Host       string
	Last, Next time.Time
	secondary  *secondaryHeap
}

type primaryHeap struct {
	M map[string]*secondaryEntry
	S []*primaryItem
}

func (q *primaryHeap) Less(i, j int) bool { return q.S[i].Next.Before(q.S[j].Next) }
func (q *primaryHeap) Top() *primaryItem  { return q.S[0] }
func (q *primaryHeap) Len() int           { return len(q.S) }
func (q *primaryHeap) Swap(i, j int) {
	hi, hj := q.S[i].Host, q.S[j].Host
	q.M[hi].idx, q.M[hj].idx = j, i
	q.S[i], q.S[j] = q.S[j], q.S[i]
}
func (q *primaryHeap) Push(x interface{}) {
	it := x.(*primaryItem)
	h := &secondaryHeap{}
	it.secondary = h
	q.M[it.Host] = &secondaryEntry{
		idx: len(q.S), secondary: h,
	}
	q.S = append(q.S, it)
}
func (q *primaryHeap) Pop() interface{} {
	n := len(q.S)
	it := q.S[n-1]
	q.S = q.S[0 : n-1]
	delete(q.M, it.Host)
	return it
}

type secondaryEntry struct {
	idx       int
	secondary *secondaryHeap
}

type secondaryHeap struct {
	S []*SchedItem
}

// Less compares compound priority of items.
func (q *secondaryHeap) Less(i, j int) bool {
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
func (q *secondaryHeap) Top() *SchedItem    { return q.S[0] }
func (q *secondaryHeap) Len() int           { return len(q.S) }
func (q *secondaryHeap) Swap(i, j int)      { q.S[i], q.S[j] = q.S[j], q.S[i] }
func (q *secondaryHeap) Push(x interface{}) { q.S = append(q.S, x.(*SchedItem)) }
func (q *secondaryHeap) Pop() interface{} {
	n := len(q.S)
	v := q.S[n-1]
	q.S = q.S[0 : n-1]
	return v
}

type MemQueue struct {
	primary           primaryHeap
	maxLen            int
	closed            bool
	chIn, chOut       chan *SchedItem
	popCond, pushCond *sync.Cond
	timer             *time.Timer
	quit              chan struct{}
	*sync.Mutex
}

func NewMemQueue(maxLen int) *MemQueue {
	q := &MemQueue{
		primary: primaryHeap{
			M: make(map[string]*secondaryEntry),
		},
		maxLen: maxLen,
		Mutex:  new(sync.Mutex),
	}
	q.popCond = sync.NewCond(q.Mutex)
	q.pushCond = sync.NewCond(q.Mutex)
	return q
}

func (q *MemQueue) Channel() (push chan<- *SchedItem, pop <-chan *SchedItem) {
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

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func (q *MemQueue) getSecondary(u *url.URL) *secondaryHeap {
	host := u.Host
	if v, ok := q.primary.M[host]; ok {
		return v.secondary
	}
	heap.Push(&q.primary, &primaryItem{
		Host: host,
	})
	return q.primary.M[host].secondary
}

func (q *MemQueue) updatePrimary(host string, d time.Duration) {
	v := q.primary.M[host]
	item := q.primary.S[v.idx]
	item.Next = maxTime(item.Last.Add(d), v.secondary.Top().Next)
	heap.Fix(&q.primary, v.idx)
}

func (q *MemQueue) Push(item *SchedItem) {
	q.Lock()
	defer q.Unlock()
	for !q.closed && q.primary.Len() >= q.maxLen {
		q.pushCond.Wait()
	}
	if q.closed {
		return
	}

	host := item.URL.Host
	h := q.getSecondary(item.URL)
	heap.Push(h, item)
	q.updatePrimary(host, item.interval())
	q.popCond.Signal()
}

// Pop will block if heap is empty or none of items should be removed at now.
func (q *MemQueue) Pop() *SchedItem {
	q.Lock()
	defer q.Unlock()

	var now time.Time
	var pi *primaryItem
	wait := false

WAIT:
	for !q.closed && (q.primary.Len() == 0 || wait) {
		q.popCond.Wait()
		wait = false
	}
	if q.closed {
		return nil
	}

	pi = q.primary.Top()
	now = time.Now()

	for pi.Next.Before(now) {
		host := pi.Host
		h := pi.secondary
		si := h.Top()
		interval := si.interval()

		if si.Next.Before(now) {
			heap.Pop(h)
			if h.Len() == 0 {
				heap.Pop(&q.primary)
			} else {
				pi.Last = now
				q.updatePrimary(host, interval)
			}
			q.pushCond.Signal()
			return si
		}
		q.updatePrimary(host, interval)
		pi = q.primary.Top()
	}

	if q.timer != nil {
		q.timer.Stop()
	}
	q.timer = time.AfterFunc(pi.Next.Sub(now), func() {
		q.Lock()
		q.popCond.Signal()
		q.Unlock()
	})
	wait = true
	goto WAIT
}

func (q *MemQueue) Close() {
	q.Lock()
	defer q.Unlock()
	q.closed = true
	q.popCond.Broadcast()
	q.pushCond.Broadcast()
}
