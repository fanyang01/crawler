package crawler

import (
	"net/url"
	"time"
)

// LinkPerPage is the rouge number of links in a HTML document.
const LinkPerPage = 32

// Link represents a link found by crawler.
type Link struct {
	URL *url.URL
	// Extra holds user-specified data.
	Extra interface{}
}

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
	Extra    interface{}
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
