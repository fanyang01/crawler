package queue

import (
	"net/url"
	"time"
)

type Item struct {
	URL   *url.URL
	Next  time.Time
	Score int
}

// PQ is priority queue, using queue.SchedItem.Next and queue.SchedItem.Score as
// compound priority. It can be operated in two modes: channel mode and
// method mode, but only one mode can be used for a single instance.
type PQ interface {
	// Channel returns a pair of channel for push and pop operations.
	Channel() (push chan<- *Item, pop <-chan *url.URL)
	// Push adds an item into queue.
	Push(*Item)
	// Pop removes an item from queue.
	Pop() *url.URL
	// Close closes queue.
	Close()
}
