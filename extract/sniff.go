package extract

import (
	"net/url"
	"regexp"

	"github.com/fanyang01/crawler/urlx"
)

const (
	SniffWindowLocation = iota
	SniffAbsoluteURLs
)

var (
	windowLocationRegexp = regexp.MustCompile(
		`(window.)?location(.href)[[:space:]]*=[[:space:]]*["'](.*?)["']`,
	)
	urlRegexp = regexp.MustCompile(
		`https?://(-\.)?([^\s/?\.#-]+\.?)+(/[^\s]*)?`,
	)
)

func windowLocation(base *url.URL, s string) (result []*url.URL) {
	matches := windowLocationRegexp.FindAllStringSubmatch(s, -1)
	for _, match := range matches {
		u, err := urlx.ParseRef(base, match[3])
		if err != nil {
			continue
		}
		result = append(result, u)
	}
	return
}

func absoluteURLs(base *url.URL, s string) (result []*url.URL) {
	matches := urlRegexp.FindAllString(s, -1)
	for _, match := range matches {
		u, err := urlx.ParseRef(base, match)
		if err != nil {
			continue
		}
		result = append(result, u)
	}
	return
}
