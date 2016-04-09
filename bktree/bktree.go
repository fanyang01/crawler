package bktree

import (
	"github.com/mfonda/simhash"
)

type Tree struct {
	fingerprint uint64
	// TODO: may use too much RAM; verify that RAM used by map is small.
	// child       [65]*Tree
	child map[int]*Tree
}

func New() *Tree {
	return &Tree{fingerprint: 0}
}

func Distance(a, b uint64) int {
	return int(simhash.Compare(a, b))
}

func (t *Tree) Has(f uint64, r int) bool {
	d := Distance(f, t.fingerprint)
	if d <= r {
		return true
	}
	var idx int
	var p *Tree
	for i := 0; i <= r; i++ {
		if p = t.child[d-i]; p != nil {
			if p.Has(f, r) {
				return true
			}
		}
		if idx = d + i; idx != d && idx <= 64 {
			if p = t.child[idx]; p != nil {
				if p.Has(f, r) {
					return true
				}
			}
		}
	}
	return false
}

func (t *Tree) Add(f uint64) {
	d := Distance(f, t.fingerprint)
	if d == 0 {
		return
	}
	if p := t.child[d]; p != nil {
		p.Add(f)
	} else {
		t.child[d] = &Tree{fingerprint: f}
	}
}
