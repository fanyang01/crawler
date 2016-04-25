// Package diskqueue implements a wait queue based on boltdb.
package diskqueue

import (
	"bytes"
	"container/list"
	"fmt"
	"log"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/boltdb/bolt"
	"github.com/fanyang01/crawler/queue"
	"github.com/fanyang01/rbtree"
)

const KeyTimeFormat = "20060102T15:04:05.000"

var (
	QueueBucket = []byte("QUEUE")
)

type element struct {
	item *queue.Item
	uid  string
}

func (e *element) key() []byte {
	// 20060102T15:04:05.000 0123456789
	b := make([]byte, 0, 32)
	b = append(b, e.item.Next.UTC().Format(KeyTimeFormat)...)
	b = append(b, ' ')
	b = append(b, e.uid...)
	return b
}

func (e *element) url() []byte {
	return []byte(e.item.URL.String())
}

type DiskQueue struct {
	limit   int    // > 0
	genID   uint32 // naive implementation
	bufSize int

	mu       sync.Mutex
	popCond  *sync.Cond
	tree     *rbtree.Tree
	dbMinKey []byte
	dbCount  int // includes bufCount
	timer    *time.Timer

	writeMu   sync.Mutex
	writeCond *sync.Cond
	flush     chan struct{}
	buf       *list.List
	bufCount  int

	chErr chan error

	db   *bolt.DB
	file string
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

func New(dbfile string, memQueueSize int) (q *DiskQueue, err error) {
	db, err := bolt.Open(dbfile, 0644, nil)
	if err != nil {
		return
	}
	var (
		dbCount  int
		dbMinKey []byte
	)
	if err = db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(QueueBucket)
		if err != nil {
			return err
		}
		if dbCount = b.Stats().KeyN; dbCount != 0 {
			dbMinKey, _ = b.Cursor().First()
		}
		return nil
	}); err != nil {
		return
	}
	q = &DiskQueue{
		tree:     rbtree.New(compare),
		limit:    memQueueSize,
		db:       db,
		file:     dbfile,
		dbCount:  dbCount,
		dbMinKey: dbMinKey,
		buf:      list.New(),
		bufSize:  128,
		chErr:    make(chan error, 1),
	}
	q.popCond = sync.NewCond(&q.mu)
	q.writeCond = sync.NewCond(&q.writeMu)
	go q.writeLoop()
	return q, nil
}

func (q *DiskQueue) writeLoop() {
	q.writeMu.Lock()
	defer q.writeMu.Unlock()

	waiting := false
	for {
		if waiting || q.bufCount == 0 {
			waiting = false
			q.writeCond.Wait()
		}
		if q.flush == nil && q.bufCount <= q.bufSize {
			waiting = true
			continue
		}
		if q.flush != nil && q.bufCount <= 0 {
			q.flush <- struct{}{}
			q.flush = nil
			waiting = true
			continue
		}
		// Write all buffered elements to DB
		err := q.db.Update(func(tx *bolt.Tx) error {
			b := tx.Bucket(QueueBucket)
			for le := q.buf.Front(); le != nil; le = le.Next() {
				if cnt, err := q.writeOne(b, le.Value); err != nil {
					return err
				} else {
					q.bufCount -= cnt
				}
			}
			return nil
		})
		if err != nil {
			if q.flush != nil {
				close(q.flush)
			}
			q.chErr <- err
			return
		}
		if q.flush != nil {
			q.flush <- struct{}{}
			q.flush = nil
		}
	}
}

func (q *DiskQueue) writeOne(b *bolt.Bucket, v interface{}) (n int, err error) {
	switch v := v.(type) {
	case *list.List:
		for le := v.Front(); le != nil; le = le.Next() {
			elem := le.Value.(*element)
			if err = b.Put(elem.key(), elem.url()); err != nil {
				return
			}
			n++
		}
	case *element:
		if err = b.Put(v.key(), v.url()); err != nil {
			return
		}
		n = 1
	}
	return
}

func (q *DiskQueue) nextID() string {
	id := atomic.AddUint32(&q.genID, 1)
	return fmt.Sprintf("%010d", id)
}

func (q *DiskQueue) Push(item *queue.Item) {
	el := &element{item: item, uid: q.nextID()}

	q.mu.Lock()
	defer func() {
		q.popCond.Signal()
		q.mu.Unlock()
	}()

	if q.dbMinKey != nil && bytes.Compare(el.key(), q.dbMinKey) > 0 {
		q.writeToBuffer(el, 1)
		return
	}
	// Now DB is empty, or each element in DB is greater than or equal to this element.
	q.tree.Insert(el)
	if q.tree.Len() <= q.limit { // Memory queue is not full.
		return
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
	q.writeToBuffer(list, list.Len())
	return
}

func (q *DiskQueue) writeToBuffer(v interface{}, cnt int) {
	q.dbCount += cnt // protected by q.mu
	q.writeMu.Lock()
	q.buf.PushBack(v)
	q.bufCount += cnt
	q.writeMu.Unlock()
}

func (q *DiskQueue) Pop() *url.URL {
	q.mu.Lock()
	defer q.mu.Unlock()

	waiting := false
	for {
		if waiting || q.tree.Len()+q.dbCount <= 0 {
			q.popCond.Wait()
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
				u := el.item.URL
				el.item.Free()
				return u
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
		q.writeMu.Lock()
		q.flush = ch
		q.writeCond.Signal()
		q.writeMu.Unlock()
		if _, ok := <-ch; !ok {
			return nil
		}

		// Fill half of the queue and ensure loading at least one item from DB.
		if n = q.limit/2 + 1; n > q.dbCount {
			n = q.dbCount
		}
		if err := q.db.Update(func(tx *bolt.Tx) error {
			b := tx.Bucket([]byte(QueueBucket))
			c := b.Cursor()
			i := 0
			for k, v := c.First(); k != nil && i < n; k, v = c.Next() {
				item := queue.NewItem()
				item.URL, _ = url.Parse(string(v))
				item.Next = timeFromKey(k)
				q.tree.Insert(&element{
					item: item,
					uid:  uidFromKey(k),
				})
				if err := b.Delete(k); err != nil {
					log.Println(err)
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
			return nil
		}
	}
}

func (q *DiskQueue) newTimer(now, future time.Time) {
	if q.timer != nil {
		q.timer.Stop()
	}
	q.timer = time.AfterFunc(future.Sub(now), func() {
		q.mu.Lock()
		q.popCond.Signal()
		q.mu.Unlock()
	})
}

func timeFromKey(k []byte) time.Time {
	i := bytes.IndexByte(k, ' ')
	t, _ := time.Parse(KeyTimeFormat, string(k[:i]))
	return t
}

func uidFromKey(k []byte) string {
	i := bytes.IndexByte(k, ' ')
	return string(k[i+1:])
}
