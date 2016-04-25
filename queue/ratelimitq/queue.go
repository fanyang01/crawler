package ratelimitq

import (
	"container/heap"
	"net/url"
	"sync"
	"time"

	"github.com/fanyang01/crawler/queue"
)

type primaryItem struct {
	Host       string
	Last, Next time.Time
	secondary  *secondaryHeap
}

// Invariant:
// 1. S[i].Next = max(S[i].Last + interval, S[i].secondary.Top)
// 2. S[i].secondary.Len > 0
// 3. len(M) = len(S)
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
	S []*queue.Item
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
func (q *secondaryHeap) Top() *queue.Item   { return q.S[0] }
func (q *secondaryHeap) Len() int           { return len(q.S) }
func (q *secondaryHeap) Swap(i, j int)      { q.S[i], q.S[j] = q.S[j], q.S[i] }
func (q *secondaryHeap) Push(x interface{}) { q.S = append(q.S, x.(*queue.Item)) }
func (q *secondaryHeap) Pop() interface{} {
	n := len(q.S)
	v := q.S[n-1]
	q.S = q.S[0 : n-1]
	return v
}

type RateLimitQueue struct {
	mu       sync.Mutex
	popCond  *sync.Cond
	pushCond *sync.Cond
	timer    *time.Timer

	chOut  chan *url.URL
	chIn   chan *queue.Item
	closed bool

	primary primaryHeap
	maxHost int
	// TODO: use a background goroutine to clean timewait periodically
	timewait map[string]time.Time
	interval func(string) time.Duration
}

func NewRateLimit(maxHost int, limit func(host string) time.Duration) *RateLimitQueue {
	if limit == nil {
		limit = func(string) time.Duration { return 0 }
	}
	q := &RateLimitQueue{
		primary: primaryHeap{
			M: make(map[string]*secondaryEntry),
		},
		maxHost:  maxHost,
		timewait: make(map[string]time.Time),
		interval: limit,
	}
	q.popCond = sync.NewCond(&q.mu)
	q.pushCond = sync.NewCond(&q.mu)
	return q
}

func (q *RateLimitQueue) Channel() (push chan<- *queue.Item, pop <-chan *url.URL) {
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

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func (q *RateLimitQueue) getSecondary(host string) *secondaryHeap {
	if v, ok := q.primary.M[host]; ok {
		return v.secondary
	}
	pi := &primaryItem{
		Host: host,
	}
	if last, ok := q.timewait[host]; ok {
		delete(q.timewait, host)
		pi.Last = last
	}
	heap.Push(&q.primary, pi)
	return q.primary.M[host].secondary
}

func (q *RateLimitQueue) updatePrimary(host string, d time.Duration) {
	v := q.primary.M[host]
	item := q.primary.S[v.idx]
	item.Next = maxTime(item.Last.Add(d), v.secondary.Top().Next)
	heap.Fix(&q.primary, v.idx)
}

func (q *RateLimitQueue) Push(item *queue.Item) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for !q.closed && q.primary.Len() >= q.maxHost {
		q.pushCond.Wait()
	}
	if q.closed {
		return
	}

	host := item.URL.Host
	h := q.getSecondary(host)
	heap.Push(h, item)
	q.updatePrimary(host, q.interval(host))
	q.popCond.Signal()
}

// Pop will block if heap is empty or none of items should be removed at now.
func (q *RateLimitQueue) Pop() *url.URL {
	q.mu.Lock()
	defer q.mu.Unlock()

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

	if pi.Next.Before(now) {
		host := pi.Host
		interval := q.interval(host)
		h := pi.secondary
		si := heap.Pop(h).(*queue.Item)

		if h.Len() == 0 {
			heap.Pop(&q.primary)
			q.timewait[host] = now
		} else {
			pi.Last = now
			q.updatePrimary(host, interval)
		}
		q.pushCond.Signal()
		return si.URL
	}

	if q.timer != nil {
		q.timer.Stop()
	}
	q.timer = time.AfterFunc(pi.Next.Sub(now), func() {
		q.mu.Lock()
		q.popCond.Signal()
		q.mu.Unlock()
	})
	wait = true
	goto WAIT
}

func (q *RateLimitQueue) Close() {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.closed = true
	q.popCond.Broadcast()
	q.pushCond.Broadcast()
}
