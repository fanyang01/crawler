package crawler

import (
	"errors"
	"net/url"
	"sync"
)

var ErrItemNotFound = errors.New("memstore: item is not found")

// Store stores all URLs.
type Store interface {
	Exist(u *url.URL) (bool, error)
	Get(u *url.URL) (*URL, error)
	GetDepth(u *url.URL) (int, error)
	GetExtra(u *url.URL) (interface{}, error)
	PutNX(u *URL) (bool, error)
	Update(u *URL) error
	UpdateExtra(u *url.URL, extra interface{}) error
	Complete(u *url.URL) error

	IncVisitCount() error
	IsFinished() (bool, error)
}

type PersistableStore interface {
	Store
	// Recover send all unfinished URLs to ch.
	Recover(ch chan<- *url.URL) error
}

type MemStore struct {
	sync.RWMutex
	m map[string]*URL

	NumURL   int32
	NumDone  int32
	NumVisit int32
	NumError int32
}

func NewMemStore() *MemStore {
	return &MemStore{
		m: make(map[string]*URL),
	}
}

func (p *MemStore) Exist(u *url.URL) (bool, error) {
	p.RLock()
	defer p.RUnlock()
	_, ok := p.m[u.String()]
	return ok, nil
}

func (p *MemStore) Get(u *url.URL) (uu *URL, err error) {
	p.RLock()
	entry, present := p.m[u.String()]
	if present {
		uu = entry.clone()
	} else {
		err = errors.New("memstore: item is not found")
	}
	p.RUnlock()
	return
}

func (p *MemStore) GetDepth(u *url.URL) (int, error) {
	p.RLock()
	defer p.RUnlock()
	if uu, ok := p.m[u.String()]; ok {
		return uu.Depth, nil
	}
	return 0, ErrItemNotFound
}

func (p *MemStore) GetExtra(u *url.URL) (interface{}, error) {
	p.RLock()
	defer p.RUnlock()
	if uu, ok := p.m[u.String()]; ok {
		return uu.Extra, nil
	}
	return nil, ErrItemNotFound
}

func (p *MemStore) PutNX(u *URL) (bool, error) {
	p.Lock()
	defer p.Unlock()
	if _, ok := p.m[u.URL.String()]; ok {
		return false, nil
	}
	p.m[u.URL.String()] = u.clone()
	p.NumURL++
	return true, nil
}

func (p *MemStore) Update(u *URL) error {
	p.Lock()
	defer p.Unlock()
	uu, ok := p.m[u.URL.String()]
	if !ok {
		return ErrItemNotFound
	}
	uu.Update(u)
	return nil
}

func (p *MemStore) UpdateExtra(u *url.URL, extra interface{}) error {
	p.Lock()
	defer p.Unlock()
	uu, ok := p.m[u.String()]
	if !ok {
		return ErrItemNotFound
	}
	uu.Extra = extra
	return nil
}

func (p *MemStore) Complete(u *url.URL) error {
	p.Lock()
	defer p.Unlock()
	uu, ok := p.m[u.String()]
	if !ok {
		return ErrItemNotFound
	}
	uu.Done = true
	p.NumDone++
	return nil
}

func (p *MemStore) IncVisitCount() error {
	p.Lock()
	p.NumVisit++
	p.Unlock()
	return nil
}

func (p *MemStore) IncErrorCount() error {
	p.Lock()
	p.NumError++
	p.Unlock()
	return nil
}

func (p *MemStore) IsFinished() (bool, error) {
	p.RLock()
	defer p.RUnlock()
	return p.NumDone >= p.NumURL, nil
}
