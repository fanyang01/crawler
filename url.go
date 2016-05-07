package crawler

import (
	"net/url"
	"time"
)

// Status of a URL.
const (
	URLStatusProcessing = iota
	URLStatusFinished
	URLStatusError
)

// URL contains metadata of a url in crawler.
type URL struct {
	Loc    url.URL
	Depth  int
	Status int

	// Can modified by Update
	Last       time.Time
	VisitCount int
	ErrorCount int
}

func (u *URL) clone() *URL {
	uu := *u
	return &uu
}

func (uu *URL) Update(u *URL) {
	uu.ErrorCount = u.ErrorCount
	uu.VisitCount = u.VisitCount
	uu.Last = u.Last
}
