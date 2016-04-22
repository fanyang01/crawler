// Package ratelimit implements a rate limiter for crawler.
package ratelimit

import (
	"net/url"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type entry struct {
	count   int
	limiter *rate.Limiter
}

// QueryFunc returns the rate limit of a host.
type QueryFunc func(host string) (inteval time.Duration, burst int)

// A Limit controls how frequently hosts are allowed to be crawled.
type Limit struct {
	mu   sync.Mutex
	host map[string]*entry

	query     QueryFunc
	updatable bool
	freq      int
}

// New creates a rate limiter. Query function will be called only once for
// each host.
func New(query QueryFunc) *Limit {
	return &Limit{
		host:  make(map[string]*entry),
		query: query,
	}
}

// NewUpdatable creates an updatable rate limiter. Query function will be
// called every freq times for each host. Only frequency(interval returned
// by query) is updatable.
func NewUpdatable(freq int, query QueryFunc) *Limit {
	return &Limit{
		host:      make(map[string]*entry),
		query:     query,
		updatable: true,
		freq:      freq,
	}
}

// Reserve returns how long the crawler should wait before crawling this
// URL.
func (l *Limit) Reserve(u *url.URL) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()

	h := u.Host
	v, ok := l.host[h]
	if !ok {
		d, burst := l.query(h)
		v = &entry{
			limiter: rate.NewLimiter(rate.Every(d), burst),
		}
		l.host[h] = v
	} else if l.updatable && v.count >= l.freq {
		d, _ := l.query(h)
		v.limiter.SetLimit(rate.Every(d))
		v.count = 0
	}
	if l.updatable {
		v.count++
	}
	return v.limiter.Reserve().Delay()
}
