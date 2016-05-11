package fingerprint

import (
	"io"
	"sync"

	"github.com/fanyang01/crawler/sample/bktree"
)

type Store struct {
	distance int
	shingle  int
	token    int

	mu   sync.Mutex
	tree *bktree.Tree
}

func NewStore(distance, shingle, token int) *Store {
	if token <= 0 {
		token = 4096
	}
	return &Store{
		distance: distance,
		shingle:  shingle,
		token:    token,
		tree:     bktree.New(),
	}
}

func (s *Store) Add(r io.Reader) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	fp, err := Compute(r, s.token, s.shingle)
	if err != nil {
		return false, err
	}
	if s.tree.Has(fp, s.distance) {
		return false, nil
	}
	s.tree.Add(fp)
	return true, nil
}
