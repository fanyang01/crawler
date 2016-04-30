package crawler

import (
	"errors"
	"io"
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
	// Recover writes all unfinished URLs to w, seperated by '\n'.
	Recover(w io.Writer) (n int, err error)
}

type MemStore struct {
	sync.RWMutex
	m map[url.URL]*URL

	URLs     int32
	Finished int32
	Ntimes   int32
	Errors   int32
}

func NewMemStore() *MemStore {
	return &MemStore{
		m: make(map[url.URL]*URL),
	}
}

func (p *MemStore) Exist(u *url.URL) (bool, error) {
	p.RLock()
	defer p.RUnlock()
	_, ok := p.m[*u]
	return ok, nil
}

func (p *MemStore) Get(u *url.URL) (uu *URL, err error) {
	p.RLock()
	entry, present := p.m[*u]
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
	if uu, ok := p.m[*u]; ok {
		return uu.Depth, nil
	}
	return 0, nil
}

func (p *MemStore) PutNX(u *URL) (bool, error) {
	p.Lock()
	defer p.Unlock()
	if _, ok := p.m[u.Loc]; ok {
		return false, nil
	}
	p.m[u.Loc] = u.clone()
	p.URLs++
	return true, nil
}

func (p *MemStore) Update(u *URL) error {
	p.Lock()
	defer p.Unlock()
	uu, ok := p.m[u.Loc]
	if !ok {
		return nil
	}
	uu.Update(u)
	return nil
}

func (p *MemStore) UpdateStatus(u *url.URL, status int) error {
	p.Lock()
	defer p.Unlock()

	uu, ok := p.m[*u]
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
