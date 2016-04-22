// Package bloom uses bloom filter to record URLs.
package bloom

import (
	"io"
	"net/url"
	"sync"

	"github.com/tylertreat/BoomFilters"
)

// PBF is persistable bloom filter.
type PBF interface {
	boom.Filter
	ReadFrom(io.Reader) (int64, error)
	WriteTo(io.Writer) (int64, error)
}

type Filter struct {
	filter PBF
	host   map[string]struct{}
	mu     sync.RWMutex
}

func NewFilter(size int, rateFP float64) *Filter {
	var ft PBF
	if size <= 0 {
		ft = boom.NewDefaultScalableBloomFilter(rateFP)
	} else {
		ft = boom.NewPartitionedBloomFilter(uint(size), rateFP)
	}
	return &Filter{
		filter: ft,
		host:   make(map[string]struct{}),
	}
}

func NewFilterWith(filter PBF) *Filter {
	return &Filter{
		filter: filter,
		host:   make(map[string]struct{}),
	}
}

func (b *Filter) Add(u *url.URL) (exist bool) {
	host := u.Host
	us := []byte(u.String())
	b.mu.Lock()
	_, ok := b.host[host]
	b.host[host] = struct{}{}
	exist = b.filter.TestAndAdd(us)
	b.mu.Unlock()
	return ok && exist
}

func (b *Filter) Exist(u *url.URL) bool {
	host := u.Host
	us := []byte(u.String())
	b.mu.RLock()
	_, ok := b.host[host]
	exist := b.filter.Test(us)
	b.mu.RUnlock()
	return ok && exist
}

func (b *Filter) ReadFrom(r io.Reader) (int64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.filter.ReadFrom(r)
}

func (b *Filter) WriteTo(w io.Writer) (int64, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.filter.WriteTo(w)
}
