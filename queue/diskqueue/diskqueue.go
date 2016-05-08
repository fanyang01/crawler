// Package diskqueue implements a wait queue based on boltdb.
package diskqueue

import (
	"bytes"
	"container/list"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/boltdb/bolt"
	"github.com/fanyang01/crawler/queue"
	"github.com/fanyang01/rbtree"
)

// TimeFormat is used to format timestamps into part of DB key.
const TimeFormat = "20060102T15:04:05.000"

// Default configuration.
var (
	DefaultBucket       = []byte("QUEUE")
	DefaultMemQueueSize = 4094
	DefaultBufSize      = 256
)

type element struct {
	item *queue.Item
	uid  string
}

func (e *element) key() []byte {
	// 20060102T15:04:05.000 0123456789
	b := make([]byte, 0, 32)
	b = append(b, e.item.Next.UTC().Format(TimeFormat)...)
	b = append(b, ' ')
	b = append(b, e.uid...)
	return b
}

func (e *element) encode() (b []byte, err error) {
	// return []byte(e.item.URL.String())
	return json.Marshal(e.item)
}

func (e *element) decode(b []byte) error {
	e.item = queue.NewItem()
	return json.Unmarshal(b, e.item)
}

// A DiskQueue can store numerous items without using too much memory.
// If the size exceeds given limit, some items will be stored on disk.
// Note it's not a reliable queue because items are write to disk temporarily.
type DiskQueue struct {
	genID uint32 // naive implementation

	mu       sync.Mutex
	cond     *sync.Cond
	tree     *rbtree.Tree
	limit    int // >= 0
	dbMinKey []byte
	dbCount  int // includes bufCount
	timer    *time.Timer
	closed   bool
	err      error

	write struct {
		mu    sync.Mutex
		cond  *sync.Cond
		buf   *list.List
		size  int // >= 0
		count int
		flush chan struct{}
		err   error
		quit  bool
	}

	db     *bolt.DB
	bucket []byte
	file   string
}

func compare(x, y interface{}) int {
	a := x.(*element)
	b := y.(*element)
	if a.item.Next.Before(b.item.Next) {
		return -1
	} else if a.item.Next.After(b.item.Next) {
		return 1
	}
	return strings.Compare(a.uid, b.uid)
}

// NewDefault creates a wait queue using the default configuration.
func NewDefault(filename string) (q queue.WaitQueue, err error) {
	return New(filename, DefaultBucket, DefaultMemQueueSize, DefaultBufSize)
}

// New creates a wait queue. 0 <= writeBufSize < memQueueSize.
// The bucket will be created if it does not exist. Otherwise,
// the bucket will be deleted and created again.
func New(filename string, bucket []byte, memQueueSize, writeBufSize int) (wq queue.WaitQueue, err error) {
	q, err := newDiskQueue(filename, bucket, memQueueSize, writeBufSize)
	if err != nil {
		return
	}
	return queue.WithChannel(q), nil
}

func newDiskQueue(filename string, bucket []byte, memQueueSize, writeBufSize int) (q *DiskQueue, err error) {
	db, err := bolt.Open(filename, 0644, nil)
	if err != nil {
		return
	}
	var (
		dbCount  int
		dbMinKey []byte
	)
	if err = db.Update(func(tx *bolt.Tx) error {
		if tx.Bucket(bucket) != nil {
			if err := tx.DeleteBucket(bucket); err != nil {
				return err
			}
		}
		if _, err := tx.CreateBucketIfNotExists(bucket); err != nil {
			return err
		}
		return nil

	}); err != nil {
		db.Close()
		return
	}

	if memQueueSize < 0 {
		memQueueSize = 0
	}
	if writeBufSize < 0 {
		writeBufSize = 0
	}
	q = &DiskQueue{
		tree:     rbtree.New(compare),
		limit:    memQueueSize,
		db:       db,
		file:     filename,
		dbCount:  dbCount,
		dbMinKey: dbMinKey,
		bucket:   bucket,
	}
	q.cond = sync.NewCond(&q.mu)

	q.write.buf = list.New()
	q.write.size = writeBufSize
	q.write.cond = sync.NewCond(&q.write.mu)
	go q.writeLoop()

	return
}

func (q *DiskQueue) writeLoop() {
	q.write.mu.Lock()
	defer q.write.mu.Unlock()

	waiting := false
	for {
		if q.write.quit {
			return
		}
		if waiting || q.write.count == 0 {
			waiting = false
			q.write.cond.Wait()
		}
		if q.write.flush == nil && q.write.count <= q.write.size {
			waiting = true
			continue
		}
		if q.write.flush != nil && q.write.count <= 0 {
			q.write.flush <- struct{}{}
			q.write.flush = nil
			waiting = true
			continue
		}
		// Write all buffered elements to DB
		if err := q.db.Update(func(tx *bolt.Tx) error {
			b := tx.Bucket(q.bucket)
			var next *list.Element
			for le := q.write.buf.Front(); le != nil; le = next {
				// Remove this element from list
				next = le.Next()
				elem := q.write.buf.Remove(le).(*element)

				v, err := elem.encode()
				if err != nil {
					return err
				}
				if err := b.Put(elem.key(), v); err != nil {
					return err
				}
				q.write.count--
			}
			return nil

		}); err != nil {
			q.write.err = err
			if q.write.flush != nil {
				close(q.write.flush)
			}
			return
		}
		if q.write.flush != nil {
			q.write.flush <- struct{}{}
			q.write.flush = nil
		}
	}
}

func (q *DiskQueue) nextID() string {
	id := atomic.AddUint32(&q.genID, 1)
	return fmt.Sprintf("%010d", id)
}

// Push implements the queue.WaitQueue interface.
func (q *DiskQueue) Push(item *queue.Item) (err error) {
	el := &element{item: item, uid: q.nextID()}

	q.mu.Lock()
	defer func() {
		q.cond.Signal()
		q.mu.Unlock()
	}()

	if q.closed {
		return
	} else if q.err != nil {
		return q.err
	}

	if q.dbMinKey != nil && bytes.Compare(el.key(), q.dbMinKey) > 0 {
		return q.writeToBuffer(el)
	}
	// Now DB is empty, or each element in DB is greater than or equal to this element.
	q.tree.Insert(el)
	if q.tree.Len() <= q.limit { // Memory queue is not full.
		return nil
	}
	// Now memory queue is full, i.e., q.tree.Len() == q.limit + 1.
	// Write half of memory queue to DB.
	var (
		n    = q.limit/2 + 1 // limit -> nwrite: 0 -> 1, 1 -> 1, 2 -> 2
		last = q.tree.Last()
		list = list.New()
		v    interface{}
	)
	for i := 0; i < n; i++ {
		prev := q.tree.Prev(last)
		v = q.tree.Delete(last)
		list.PushFront(v)
		last = prev
	}
	minKey := v.(*element).key()
	if q.dbMinKey == nil || bytes.Compare(minKey, q.dbMinKey) < 0 {
		q.dbMinKey = minKey
	}
	q.err = q.writeToBuffer(list)
	return q.err
}

func (q *DiskQueue) writeToBuffer(v interface{}) error {
	q.write.mu.Lock()
	defer q.write.mu.Unlock()

	if q.write.err != nil {
		return q.write.err
	}
	var cnt int
	switch v := v.(type) {
	case *element:
		q.write.buf.PushBack(v)
		cnt = 1
	case *list.List:
		q.write.buf.PushBackList(v)
		cnt = v.Len()
	}
	q.write.count += cnt
	q.write.cond.Signal()

	q.dbCount += cnt // protected by q.mu
	return nil
}

// Push implements the queue.WaitQueue interface.
func (q *DiskQueue) Pop() (item *queue.Item, err error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	waiting := false
	for {
		if q.closed {
			return
		} else if q.err != nil {
			return nil, q.err
		}

		if waiting || q.tree.Len()+q.dbCount <= 0 {
			q.cond.Wait()
			waiting = false
			continue
		}
		if q.tree.Len() != 0 {
			var (
				node = q.tree.First()
				el   = node.Value().(*element)
				now  = time.Now()
			)
			if el.item.Next.Before(now) {
				q.tree.Delete(node)
				item = el.item
				return
			}
			q.newTimer(now, el.item.Next)
			waiting = true
			continue
		}

		// Now q.tree.Len() == 0 and q.dbMinKey != nil
		var (
			next = timeFromKey(q.dbMinKey)
			now  = time.Now()
			n    int
		)
		if next.After(now) {
			q.newTimer(now, next)
			waiting = true
			continue
		}

		// Ask writing goroutine to write all buffered items.
		ch := make(chan struct{}, 1)
		q.write.mu.Lock()
		q.write.flush = ch
		q.write.cond.Signal()
		q.write.mu.Unlock()
		if _, ok := <-ch; !ok {
			q.write.mu.Lock()
			err = q.write.err
			q.write.mu.Unlock()
			q.err = err
			return
		}

		// Fill half of the queue and ensure loading at least one item from DB.
		if n = q.limit/2 + 1; n > q.dbCount {
			n = q.dbCount
		}
		if err = q.db.Update(func(tx *bolt.Tx) error {
			b := tx.Bucket(q.bucket)
			c := b.Cursor()
			i := 0
			for k, v := c.First(); k != nil && i < n; k, v = c.Next() {
				elem := &element{}
				if err := elem.decode(v); err != nil {
					return err
				}
				elem.uid = uidFromKey(k)
				q.tree.Insert(elem)
				if err := b.Delete(k); err != nil {
					return err
				}
				i++
			}
			// Update dbCount and dbMinKey
			if q.dbCount = b.Stats().KeyN; q.dbCount <= 0 {
				q.dbMinKey = nil
			} else {
				q.dbMinKey, _ = b.Cursor().First()
			}
			return nil

		}); err != nil {
			q.err = err
			return
		}
	}
}

func (q *DiskQueue) newTimer(now, future time.Time) {
	if q.timer != nil {
		q.timer.Stop()
	}
	q.timer = time.AfterFunc(future.Sub(now), func() {
		q.mu.Lock()
		q.cond.Signal()
		q.mu.Unlock()
	})
}

func timeFromKey(k []byte) time.Time {
	i := bytes.IndexByte(k, ' ')
	t, _ := time.Parse(TimeFormat, string(k[:i]))
	return t
}

func uidFromKey(k []byte) string {
	i := bytes.IndexByte(k, ' ')
	return string(k[i+1:])
}

// Push implements the queue.WaitQueue interface.
func (q *DiskQueue) Close() error {
	q.mu.Lock()
	q.closed = true
	q.cond.Signal()
	q.mu.Unlock()

	q.write.mu.Lock()
	q.write.quit = true
	q.write.cond.Signal()
	q.write.mu.Unlock()

	return q.db.Close()
}
