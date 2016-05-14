// Package ratelimitq provides a strict host-based rate limit queue.
package ratelimitq

import (
	"container/heap"
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
	M map[string]int
	S []*primaryItem
}

func (q *primaryHeap) Less(i, j int) bool { return q.S[i].Next.Before(q.S[j].Next) }
func (q *primaryHeap) Top() *primaryItem  { return q.S[0] }
func (q *primaryHeap) Len() int           { return len(q.S) }
func (q *primaryHeap) Swap(i, j int) {
	hi, hj := q.S[i].Host, q.S[j].Host
	q.M[hi], q.M[hj] = j, i
	q.S[i], q.S[j] = q.S[j], q.S[i]
}
func (q *primaryHeap) Push(x interface{}) {
	it := x.(*primaryItem)
	q.M[it.Host] = len(q.S)
	q.S = append(q.S, it)
}
func (q *primaryHeap) Pop() interface{} {
	n := len(q.S)
	it := q.S[n-1]
	q.S = q.S[0 : n-1]
	delete(q.M, it.Host)
	return it
}

type Secondary interface {
	Top(host string) (*queue.Item, error)
	Len(host string) (int, error)
	Push(host string, item *queue.Item) error
	Pop(host string) (*queue.Item, error)
	Close() error
}

type secondaryHeap struct {
	M map[string]*queue.Heap
}

func newSecondaryHeap() *secondaryHeap {
	return &secondaryHeap{
		M: make(map[string]*queue.Heap),
	}
}

func (q *secondaryHeap) Top(host string) (*queue.Item, error) {
	return q.M[host].Top(), nil
}
func (q *secondaryHeap) Len(host string) (int, error) {
	if h, ok := q.M[host]; !ok {
		return 0, nil
	} else {
		return h.Len(), nil
	}
}
func (q *secondaryHeap) Push(host string, item *queue.Item) error {
	h, ok := q.M[host]
	if !ok {
		h = &queue.Heap{}
		q.M[host] = h
	}
	heap.Push(h, item)
	return nil
}
func (q *secondaryHeap) Pop(host string) (*queue.Item, error) {
	h := q.M[host]
	item := heap.Pop(h).(*queue.Item)
	if h.Len() == 0 {
		delete(q.M, host)
	}
	return item, nil
}
func (q *secondaryHeap) Close() error { return nil }

type RateLimitQueue struct {
	mu       sync.Mutex
	popCond  *sync.Cond
	pushCond *sync.Cond
	timer    *time.Timer
	closed   bool

	primary   primaryHeap
	secondary Secondary
	maxHost   int
	// TODO: use a background goroutine to clean timewait periodically
	timewait map[string]time.Time
	interval func(string) time.Duration
	err      error
}

type Option struct {
	MaxHosts  int
	Secondary Secondary
	Limit     func(host string) time.Duration
}

// NewWaitQueue creates a rate limit wait queue.
func NewWaitQueue(opt *Option) queue.WaitQueue {
	return queue.WithChannel(New(opt))
}

func New(opt *Option) *RateLimitQueue {
	if opt == nil {
		opt = &Option{}
	}
	if opt.Secondary == nil {
		opt.Secondary = newSecondaryHeap()
	}
	if opt.Limit == nil {
		opt.Limit = func(string) time.Duration { return 0 }
	}

	q := &RateLimitQueue{
		primary: primaryHeap{
			M: make(map[string]int),
		},
		maxHost:   opt.MaxHosts,
		secondary: opt.Secondary,
		interval:  opt.Limit,
		timewait:  make(map[string]time.Time),
	}
	q.popCond = sync.NewCond(&q.mu)
	q.pushCond = sync.NewCond(&q.mu)
	return q
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func (q *RateLimitQueue) create(host string) {
	if _, ok := q.primary.M[host]; ok {
		return
	}
	pi := &primaryItem{
		Host: host,
	}
	if last, ok := q.timewait[host]; ok {
		delete(q.timewait, host)
		pi.Last = last
	}
	heap.Push(&q.primary, pi)
}

func (q *RateLimitQueue) update(host string, d time.Duration) error {
	idx := q.primary.M[host]
	item := q.primary.S[idx]
	top, err := q.secondary.Top(host)
	if err != nil {
		return err
	}
	item.Next = maxTime(item.Last.Add(d), top.Next)
	heap.Fix(&q.primary, idx)
	return nil
}

func (q *RateLimitQueue) Push(item *queue.Item) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	for q.err == nil && !q.closed &&
		(q.maxHost > 0 && q.primary.Len() >= q.maxHost) {
		q.pushCond.Wait()
	}
	if q.err != nil {
		return q.err
	} else if q.closed {
		return queue.ErrPushClosed
	}

	host := item.URL.Host
	q.create(host)
	q.secondary.Push(host, item)
	if q.err = q.update(host, q.interval(host)); q.err != nil {
		return q.err
	}
	q.popCond.Signal()
	return nil
}

// Pop will block if heap is empty or none of items should be removed at now.
// It will return nil without error if the queue is closed.
func (q *RateLimitQueue) Pop() (item *queue.Item, err error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	var now time.Time
	var pi *primaryItem
	wait := false

WAIT:
	for q.err == nil && !q.closed && (q.primary.Len() == 0 || wait) {
		q.popCond.Wait()
		wait = false
	}
	if q.err != nil {
		return nil, q.err
	} else if q.closed {
		return nil, nil
	}

	pi = q.primary.Top()
	now = time.Now()

	if pi.Next.Before(now) {
		var (
			host     = pi.Host
			interval = q.interval(host)
			len      int
		)
		if item, q.err = q.secondary.Pop(host); q.err != nil {
			return nil, q.err
		} else if len, q.err = q.secondary.Len(host); q.err != nil {
			return nil, q.err
		} else if len <= 0 {
			heap.Pop(&q.primary)
			q.timewait[host] = now
		} else {
			pi.Last = now
			q.update(host, interval)
		}
		q.pushCond.Signal()
		return
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

func (q *RateLimitQueue) Close() error {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.closed = true
	q.popCond.Broadcast()
	q.pushCond.Broadcast()
	return nil
}
