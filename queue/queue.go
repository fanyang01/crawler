// Package queue defines the wait queue interface.
package queue

import (
	"net/url"
	"time"
)

// Item is the item in wait queue.
type Item struct {
	URL   *url.URL
	Next  time.Time // next time to crawl
	Score int       // optional for implementation
}

// WaitQueue should be a priority queue, using Item.Next and optional
// Item.Score as priority. It can be operated in two modes: channel mode
// and method mode, but only one mode will be used for a single instance.
type WaitQueue interface {
	// Channel returns a pair of channel for push and pop operations.
	Channel() (push chan<- *Item, pop <-chan *url.URL)
	// Push adds an item to queue.
	Push(*Item)
	// Pop removes an item from queue.
	Pop() *url.URL
	// Close closes queue.
	Close()
}
