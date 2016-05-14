// Package diskheap implements a priority queue based on boltdb.
package diskheap

import (
	"container/list"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/boltdb/bolt"
	"github.com/fanyang01/crawler/queue"
)

var DefaultPrefix = []byte("HEAP")

// TimeFormat is used to format timestamps into part of DB key.
const TimeFormat = "20060102T15:04:05.000"

type element struct {
	item *queue.Item
	uid  string
}

func (e *element) priority() int {
	if e.item.Score >= 999 {
		return 0
	} else if e.item.Score <= 0 {
		return 999
	}
	return 999 - e.item.Score
}

func (e *element) key() []byte {
	// timestamp priority uid
	// 20060102T15:04:05.000 012 0123456789
	b := make([]byte, 0, 36)
	b = append(b, e.item.Next.UTC().Format(TimeFormat)...)
	b = append(b, ' ')
	b = append(b, fmt.Sprintf("%03d", e.priority())...)
	b = append(b, ' ')
	b = append(b, e.uid...)
	return b
}

func (e *element) encode() (b []byte, err error) {
	return json.Marshal(e.item)
}

func (e *element) decode(b []byte) error {
	e.item = queue.NewItem()
	return json.Unmarshal(b, e.item)
}

// A DiskHeap stores items in a boltdb bucket. Note it's not reliable
// because items are write to disk temporarily.
type Heap struct {
	genID    uint32 // naive implementation
	db       *bolt.DB
	bucket   []byte
	writebuf *list.List
	bufsize  int
	min      *queue.Item
	count    int
}

// New creates a wait queue.
// The bucket will be created if it does not exist. Otherwise,
// the bucket will be deleted and created again.
func NewHeap(db *bolt.DB, bucket []byte, bufsize int) (h *Heap, err error) {
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
	h = &Heap{
		db:       db,
		count:    0,
		min:      nil,
		writebuf: list.New(),
		bufsize:  bufsize,
		bucket:   bucket,
	}
	return
}

func (h *Heap) nextID() string {
	id := atomic.AddUint32(&h.genID, 1)
	return fmt.Sprintf("%010d", id)
}

func (h *Heap) Push(item *queue.Item) error {
	el := &element{item: item, uid: h.nextID()}
	if h.min == nil || item.Next.Before(h.min.Next) {
		h.min = item
	}
	h.writebuf.PushBack(el)
	if h.writebuf.Len() > h.bufsize {
		if err := h.writeAll(); err != nil {
			return err
		}
	}
	h.count++
	return nil
}

func (h *Heap) writeAll() error {
	var next *list.Element
	if err := h.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(h.bucket)
		for e := h.writebuf.Front(); e != nil; e = next {
			next = e.Next()
			el := h.writebuf.Remove(e).(*element)
			k := el.key()
			v, err := el.encode()
			if err != nil {
				return err
			}
			if err := b.Put(k, v); err != nil {
				return err
			}
		}
		return nil

	}); err != nil {
		return err
	}
	return nil
}

func (h *Heap) Pop() (item *queue.Item, err error) {
	// NOTE: don't use bucket stats after updating bucket
	// https://github.com/boltdb/bolt/issues/275
	if h.writebuf.Len() > 0 {
		if err = h.writeAll(); err != nil {
			return
		}
	}
	elem := &element{}
	err = h.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(h.bucket)
		cur := b.Cursor()
		if cur == nil {
			return errors.New("diskheap: broken DB stats: pop on empty bucket")
		}
		_, v := cur.First()
		if err := elem.decode(v); err != nil {
			return err
		}

		item = elem.item

		if err := cur.Delete(); err != nil {
			return err
		}
		// NOTE: don't use b.Stats().KeyN
		if h.count--; h.count <= 0 {
			h.min = nil
		} else {
			_, v := b.Cursor().First()
			if err := elem.decode(v); err != nil {
				return err
			}
			h.min = elem.item
		}
		return nil
	})
	return
}

func (h *Heap) Top() (*queue.Item, error) { return h.min, nil }
func (h *Heap) Close() error              { return nil }
func (h *Heap) Len() (int, error)         { return h.count, nil }

type DiskHeap struct {
	prefix  []byte
	bufsize int
	db      *bolt.DB
	m       map[string]*Heap
}

func New(db *bolt.DB, prefix []byte, bufPerHost int) *DiskHeap {
	if prefix == nil {
		prefix = DefaultPrefix
	}
	return &DiskHeap{
		db:      db,
		prefix:  prefix,
		bufsize: bufPerHost,
		m:       make(map[string]*Heap),
	}
}

func (q *DiskHeap) Top(host string) (*queue.Item, error) {
	return q.m[host].Top()
}
func (q *DiskHeap) Len(host string) (int, error) {
	if h, ok := q.m[host]; !ok {
		return 0, nil
	} else {
		return h.Len()
	}
}
func (q *DiskHeap) Push(host string, item *queue.Item) error {
	h, ok := q.m[host]
	var err error
	if !ok {
		bucket := make([]byte, 0, len(q.prefix)+len(host))
		bucket = append(bucket, q.prefix...)
		bucket = append(bucket, host...)
		if h, err = NewHeap(q.db, bucket, q.bufsize); err != nil {
			return err
		}
		q.m[host] = h
	}
	return h.Push(item)
}

func (q *DiskHeap) Pop(host string) (*queue.Item, error) {
	h := q.m[host]
	item, err := h.Pop()
	if err == nil {
		var len int
		if len, err = h.Len(); err == nil && len == 0 {
			delete(q.m, host)
		}
	}
	return item, err
}

func (q *DiskHeap) Close() error { return nil }
