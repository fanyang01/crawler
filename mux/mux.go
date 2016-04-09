/*
Package mux provides an implementation of crawler.Controller.

This package uses patterns to register configurations and handlers.

Supported patterns:

	- exact match: prefixed by "= ",
	- wildcard: containing any number of wildcard character "*",
	- regular expressions, prefixed by "~ ",
	- skipping regular expression: wildcard patterns prefixed by "^~ ".

For example, "= http://example.org" only matches "http://example.org",
while "http://example.org/*" and "~ http://example.org/.*" match all urls
prefixed by "http://example.org/".

Search algorithm:

	1. Exact match dictionary is firstly checked. If a match is found,
	terminate the search.
	2. Wildcard trie is checked to find a most precise match. If it's a
	skipping regular expression pattern, the search is terminated.
	3. Regular expressions are checked in sequential order. If a match is
	found, terminate the search.
	4. If no regular expression is matched, use the result of step 2.
*/
package mux

import (
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/fanyang01/crawler"
	"github.com/fanyang01/glob"
)

const (
	ExactPrefix = "= "
	RegexPrefix = "~ "
	SkipPrefix  = "^~ "
)

// Matcher is a url matcher.
type Matcher struct {
	exact map[string]interface{}
	trie  *glob.Trie
	regex []struct {
		re *regexp.Regexp
		v  interface{}
	}
}

type skipMatcher struct {
	pattern string
	v       interface{}
}

// NewMatcher creates a new matcher.
func NewMatcher() *Matcher {
	return &Matcher{
		exact: make(map[string]interface{}),
		trie:  glob.NewTrie(),
	}
}

// Add adds a pattern and associated value to the matcher.
func (m *Matcher) Add(pattern string, v interface{}) error {
	if strings.HasPrefix(pattern, ExactPrefix) {
		s := strings.TrimPrefix(pattern, ExactPrefix)
		m.exact[s] = v
		return nil
	}
	if strings.HasPrefix(pattern, RegexPrefix) {
		s := strings.TrimPrefix(pattern, RegexPrefix)
		re, err := regexp.Compile(s)
		if err != nil {
			return err
		}
		m.regex = append(m.regex, struct {
			re *regexp.Regexp
			v  interface{}
		}{
			re: re,
			v:  v,
		})
		return nil
	}
	if strings.HasPrefix(pattern, SkipPrefix) {
		s := strings.TrimPrefix(pattern, SkipPrefix)
		m.trie.Add(s, &skipMatcher{
			pattern: pattern,
			v:       v,
		})
		return nil
	}
	m.trie.Add(pattern, v)
	return nil
}

// Get looks up a pattern matching s and returns the value associated with it.
func (m *Matcher) Get(s string) (v interface{}, ok bool) {
	if v, ok = m.exact[s]; ok {
		return
	}
	if v, ok = m.trie.Lookup(s); ok {
		if m, skip := v.(*skipMatcher); skip {
			return m.v, true
		}
	}
	for _, r := range m.regex {
		if match := r.re.MatchString(s); match {
			return r.v, true
		}
	}
	return
}

const (
	muxFILTER = iota
	muxPREPARE
	muxREQTYPE
	muxHANDLE
	muxNOFOLLOW
	muxSCORE
	muxINTERVAL
	muxFREQ
	muxDEPTH
	muxLEN
)

// Mux is a multiplexer.
type Mux struct {
	crawler.NopController
	matcher [muxLEN]*Matcher
}

// NewMux creates an initialized multiplexer.
func NewMux() *Mux {
	mux := &Mux{}
	for i := 0; i < len(mux.matcher); i++ {
		mux.matcher[i] = NewMatcher()
	}
	return mux
}

type (
	// Preparer configures a request before it is actually made.
	Preparer interface {
		Prepare(*crawler.Request)
	}
	// Handler handles the response.
	Handler interface {
		Handle(*crawler.Response)
	}
	// PreparerFunc configures a request before it is actually made.
	PreparerFunc func(*crawler.Request)
	// HandlerFunc handles the response.
	HandlerFunc func(*crawler.Response)
)

// Prepare implements Preparer.
func (f PreparerFunc) Prepare(req *crawler.Request) { f(req) }

// Handle implements Handler.
func (f HandlerFunc) Handle(resp *crawler.Response) { f(resp) }

// Allow specifies that urls matching pattern should be processed.
func (mux *Mux) Allow(pattern string) {
	mux.matcher[muxFILTER].Add(pattern, true)
}

// Disallow specifies that urls matching pattern should not be processed.
// It's the default behavior.
func (mux *Mux) Disallow(pattern string) {
	mux.matcher[muxFILTER].Add(pattern, false)
}

// DoNotFollow tells crawler not to follow links on pages whose url matches pattern.
// The default behavior is to follow links.
func (mux *Mux) DoNotFollow(pattern string) {
	mux.matcher[muxNOFOLLOW].Add(pattern, false)
}

// SetScore sets score for urls matching pattern.
func (mux *Mux) SetScore(pattern string, score int) {
	mux.matcher[muxSCORE].Add(pattern, score)
}

// SetFreq tells crawler the maximum number of times a url should be crawled.
func (mux *Mux) SetFreq(pattern string, n int) {
	mux.matcher[muxFREQ].Add(pattern, n)
}

// SetMaxDepth limits the crawler to stop at given depth.
func (mux *Mux) SetMaxDepth(pattern string, depth int) {
	mux.matcher[muxDEPTH].Add(pattern, depth)
}

// SetHostInterval tells crawler the interval between two visiting to a site.
// Note each host mantains a independent timer.
func (mux *Mux) SetHostInterval(pattern string, d time.Duration) {
	mux.matcher[muxINTERVAL].Add(pattern, d)
}

// Dynamic tells crawler that a url corresponds to a dynamic page.
func (mux *Mux) Dynamic(pattern string) {
	mux.matcher[muxREQTYPE].Add(pattern, crawler.ReqDynamic)
}

// Static tells crawler that a url corresponds to a static page.
// It's the default behavior.
func (mux *Mux) Static(pattern string) {
	mux.matcher[muxREQTYPE].Add(pattern, crawler.ReqStatic)
}

// AddPreparer registers p to set requests whose url matches pattern.
func (mux *Mux) AddPreparer(pattern string, p Preparer) {
	mux.matcher[muxPREPARE].Add(pattern, p)
}

// AddPrepareFunc registers f to set requests whose url matches pattern.
func (mux *Mux) AddPrepareFunc(pattern string, f func(*crawler.Request)) {
	mux.AddPreparer(pattern, PreparerFunc(f))
}

// AddHandler registers h to handle responses whose url matches pattern.
func (mux *Mux) AddHandler(pattern string, h Handler) {
	mux.matcher[muxHANDLE].Add(pattern, h)
}

// AddHandleFunc registers f to handle responses whose url matches pattern.
func (mux *Mux) AddHandleFunc(pattern string, f func(*crawler.Response)) {
	mux.AddHandler(pattern, HandlerFunc(f))
}

// Prepare implements Controller.
func (mux *Mux) Prepare(req *crawler.Request) {
	url := req.URL.String()
	if t, ok := mux.matcher[muxREQTYPE].Get(url); ok {
		req.Type = t.(crawler.RequestType)
	}
	if f, ok := mux.matcher[muxPREPARE].Get(url); ok {
		f.(Preparer).Prepare(req)
	}
}

// Handle implements Controller.
func (mux *Mux) Handle(resp *crawler.Response) []*crawler.Link {
	url := resp.NewURL.String()
	if f, ok := mux.matcher[muxHANDLE].Get(url); ok {
		f.(Handler).Handle(resp)
	}
	return nil
}

// Follow implements Controller.
func (mux *Mux) Follow(u *url.URL, depth int) bool {
	if _, ok := mux.matcher[muxNOFOLLOW].Get(u.String()); ok {
		return false
	}
	if max, ok := mux.matcher[muxDEPTH].Get(u.String()); ok {
		if depth >= max.(int) {
			return false
		}
	}
	return true
}

// Schedule implements Controller.
func (mux *Mux) Schedule(u *crawler.URL) (score int, at time.Time, done bool) {
	url := u.Loc.String()
	if t, ok := mux.matcher[muxFREQ].Get(url); ok {
		if u.Visited.Count >= t.(int) {
			done = true
			return
		}
	} else if u.Visited.Count >= 1 {
		done = true
		return
	}
	if sc, ok := mux.matcher[muxSCORE].Get(url); ok {
		score = sc.(int)
	}
	return
}

// Accept implements Controller.
func (mux *Mux) Accept(link *crawler.Link) bool {
	if ac, ok := mux.matcher[muxFILTER].Get(link.URL.String()); ok {
		return ac.(bool)
	}
	return false
}

// Interval implements Controller.
func (mux *Mux) Interval(host string) time.Duration {
	if d, ok := mux.matcher[muxINTERVAL].Get(host); ok {
		return d.(time.Duration)
	}
	return 0
}
