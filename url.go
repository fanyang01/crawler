package crawler

import (
	"net/url"
	"time"

	"golang.org/x/net/context"
)

type ctxURL struct {
	ctx context.Context
	url *url.URL
}

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
	NumRetry int
}

func (u *URL) clone() *URL {
	uu := *u
	return &uu
}

func (u *URL) Update(uu *URL) {
	u.NumRetry = uu.NumRetry
	u.NumVisit = uu.NumVisit
	u.Last = uu.Last
	u.Status = uu.Status
}
