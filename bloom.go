package crawler

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

type BloomFilter struct {
	Store
	filter PBF
	host   map[string]struct{}
	mu     sync.RWMutex
}

func NewBloomFilter(size int, rateFP float64) *BloomFilter {
	var ft PBF
	if size <= 0 {
		ft = boom.NewDefaultScalableBloomFilter(rateFP)
	} else {
		ft = boom.NewPartitionedBloomFilter(uint(size), rateFP)
	}
	return &BloomFilter{
		filter: ft,
		host:   make(map[string]struct{}),
	}
}

func NewBloomFilterWith(filter PBF) *BloomFilter {
	return &BloomFilter{
		filter: filter,
		host:   make(map[string]struct{}),
	}
}

func (b *BloomFilter) Add(u *url.URL) (exist bool) {
	host := u.Host
	us := []byte(u.String())
	b.mu.Lock()
	_, ok := b.host[host]
	b.host[host] = struct{}{}
	exist = b.filter.TestAndAdd(us)
	b.mu.Unlock()
	return ok && exist
}

func (b *BloomFilter) Exist(u *url.URL) bool {
	host := u.Host
	us := []byte(u.String())
	b.mu.RLock()
	_, ok := b.host[host]
	exist := b.filter.Test(us)
	b.mu.RUnlock()
	return ok && exist
}

func (b *BloomFilter) ReadFrom(r io.Reader) (int64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.filter.ReadFrom(r)
}

func (b *BloomFilter) WriteTo(w io.Writer) (int64, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.filter.WriteTo(w)
}
