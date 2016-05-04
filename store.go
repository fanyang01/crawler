package crawler

import (
	"errors"
	"net/url"
	"sync"
)

// Store stores all URLs.
type Store interface {
	Exist(u *url.URL) (bool, error)
	Get(u *url.URL) (*URL, error)
	GetDepth(u *url.URL) (int, error)
	PutNX(u *URL) (bool, error)
	Update(u *URL) error
	UpdateStatus(u *url.URL, status int) error

	IncVisitCount() error
	IncErrorCount() error
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

	URLs     int32
	Finished int32
	Ntimes   int32
	Errors   int32
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
	return 0, nil
}

func (p *MemStore) PutNX(u *URL) (bool, error) {
	p.Lock()
	defer p.Unlock()
	if _, ok := p.m[u.Loc.String()]; ok {
		return false, nil
	}
	p.m[u.Loc.String()] = u.clone()
	p.URLs++
	return true, nil
}

func (p *MemStore) Update(u *URL) error {
	p.Lock()
	defer p.Unlock()
	uu, ok := p.m[u.Loc.String()]
	if !ok {
		return nil
	}
	uu.Update(u)
	return nil
}

func (p *MemStore) UpdateStatus(u *url.URL, status int) error {
	p.Lock()
	defer p.Unlock()

	uu, ok := p.m[u.String()]
	if !ok {
		return nil
	}
	uu.Status = status
	switch status {
	case URLStatusFinished, URLStatusError:
		p.Finished++
	}
	return nil
}

func (s *MemStore) IncVisitCount() error {
	s.Lock()
	s.Ntimes++
	s.Unlock()
	return nil
}

func (s *MemStore) IncErrorCount() error {
	s.Lock()
	s.Errors++
	s.Unlock()
	return nil
}

func (s *MemStore) IsFinished() (bool, error) {
	s.RLock()
	defer s.RUnlock()
	return s.Finished >= s.URLs, nil
}
