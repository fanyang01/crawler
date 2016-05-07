package crawler

import (
	"net/url"
	"time"
)

// perPage is the rouge number of links in a HTML document.
const perPage = 64

// Status of a URL.
const (
	URLStatusProcessing = iota
	URLStatusFinished
	URLStatusError
)

// URL holds the metadata of a URL.
type URL struct {
	url.URL
	Depth    int
	Done     bool
	Last     time.Time
	Status   int
	NumVisit int
	NumError int
}

func (u *URL) clone() *URL {
	uu := *u
	return &uu
}

func (u *URL) Update(uu *URL) {
	u.NumError = uu.NumError
	u.NumVisit = uu.NumVisit
	u.Last = uu.Last
	u.Status = uu.Status
}
