package extract

import (
	"net/url"
	"path"
	"regexp"
	"strings"

	"github.com/gobwas/glob"
)

// A Matcher matches a URL and returns true if it is accepted.
type Matcher interface {
	Match(u *url.URL) bool
}

// Pattern matches URLs.
//
// Genenal rules:
//
//     - An item is accepted by a matcher only if it was not rejected by
//       rejection rules and was accepted by the accepting rules.
//     - Rejection rules have higher priority than accepting rules.
//     - Rules in a rule list are matched in order and logic-ORed.
//     - Different matchers are matched in source order.
//     - Rejection between different matchers is logic-OR short-circuit.
//     - A rule is either a plain glob pattern(*.html) or a regular expression
//       surrounded by slash(/regexp/).
//     - Empty rule list means accept any item.
type Pattern struct {
	// URL matcher
	Accept []string
	Reject []string

	// Host matcher
	Host        []string
	ExcludeHost []string

	// Dir matcher
	Dir        []string
	ExcludeDir []string

	// Filename matcher
	File        []string
	ExcludeFile []string
}

type matcher interface {
	Match(string) bool
}

type regex regexp.Regexp

func (r *regex) Match(s string) bool {
	return (*regexp.Regexp)(r).MatchString(s)
}

type plain string

func (p plain) Match(s string) bool {
	return string(p) == s
}

type pattern struct {
	// URL matcher
	Accept []matcher
	Reject []matcher

	// Host matcher
	Host        []matcher
	ExcludeHost []matcher

	// Dir matcher
	Dir        []matcher
	ExcludeDir []matcher

	// Filename matcher
	File        []matcher
	ExcludeFile []matcher
}

func (p *pattern) Match(u *url.URL) bool {
	f := func(s string, reject, accept []matcher) bool {
		for _, rule := range reject {
			if rule.Match(s) {
				return false
			}
		}
		if len(accept) == 0 {
			return true
		}
		for _, rule := range accept {
			if rule.Match(s) {
				return true
			}
		}
		return false
	}
	us, uh, up := u.String(), u.Host, u.EscapedPath()
	dir, file := path.Split(up)
	return f(us, p.Reject, p.Accept) &&
		f(uh, p.ExcludeHost, p.Host) &&
		f(dir, p.ExcludeDir, p.Dir) &&
		f(file, p.ExcludeFile, p.File)
}

// MustCompiles will panic if it fails to compile the pattern.
func MustCompile(p *Pattern) Matcher {
	m, err := Compile(p)
	if err != nil {
		panic(err)
	}
	return m
}

// Compile compiles a pattern for future use.
func Compile(p *Pattern) (Matcher, error) {
	ret := &pattern{}
	if p == nil {
		return ret, nil
	}
	f := func(slice []string, sep rune) (result []matcher, err error) {
		result = make([]matcher, 0, len(slice))
		for _, s := range slice {
			var (
				ok  bool
				err error
				r   *regexp.Regexp
				g   glob.Glob
			)
			if s == "" {
				result = append(result, plain(""))
				continue
			}
			if s, ok = isRegex(s); ok {
				if r, err = regexp.Compile(s); err != nil {
					return nil, err
				}
				result = append(result, (*regex)(r))
				continue
			}
			if sep != 0 {
				g, err = glob.Compile(s, sep)
			} else {
				g, err = glob.Compile(s)
			}
			if err != nil {
				return nil, err
			}
			result = append(result, g)
		}
		return result, nil
	}
	var err error
	set := func(dst *[]matcher, src []string, sep rune) bool {
		var result []matcher
		if result, err = f(src, sep); err != nil {
			return false
		}
		*dst = result
		return true
	}
	if set(&ret.Accept, p.Accept, '/') &&
		set(&ret.Reject, p.Reject, '/') &&
		set(&ret.Host, p.Host, '.') &&
		set(&ret.ExcludeHost, p.ExcludeHost, '.') &&
		set(&ret.Dir, p.Dir, '/') &&
		set(&ret.ExcludeDir, p.ExcludeDir, '/') &&
		set(&ret.File, p.File, 0) &&
		set(&ret.ExcludeFile, p.ExcludeFile, 0) {

		return ret, nil
	}
	return nil, err
}

func isRegex(s string) (string, bool) {
	if strings.HasPrefix(s, "/") && strings.HasSuffix(s, "/") {
		return strings.TrimPrefix(strings.TrimSuffix(s, "/"), "/"), true
	}
	return s, false
}
