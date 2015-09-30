package crawler

import (
	"net/url"
	"strings"
)

// this function is a copy of (net/url).resolvePath
func resolvePath(base, ref string) string {
	var full string
	if ref == "" {
		full = base
	} else if ref[0] != '/' {
		i := strings.LastIndex(base, "/")
		full = base[:i+1] + ref
	} else {
		full = ref
	}
	if full == "" {
		return ""
	}
	var dst []string
	src := strings.Split(full, "/")
	for _, elem := range src {
		switch elem {
		case ".":
			// drop
		case "..":
			if len(dst) > 0 {
				dst = dst[:len(dst)-1]
			}
		default:
			dst = append(dst, elem)
		}
	}
	if last := src[len(src)-1]; last == "." || last == ".." {
		// Add final slash to the joined path.
		dst = append(dst, "")
	}
	return "/" + strings.TrimLeft(strings.Join(dst, "/"), "/")
}

// modified from package net/url
func ResolveReference(base, ref, dst *url.URL) {
	// dst := *ref
	dst.Scheme = ref.Scheme
	dst.Opaque = ref.Opaque
	dst.User = ref.User
	dst.Host = ref.Host
	dst.Path = ref.Path
	dst.RawPath = ref.RawPath
	dst.RawQuery = ref.RawQuery
	dst.Fragment = ref.Fragment

	if ref.Scheme == "" {
		dst.Scheme = base.Scheme
	}
	if ref.Scheme != "" || ref.Host != "" || ref.User != nil {
		// The "absoluteURI" or "net_path" cases.
		dst.Path = resolvePath(ref.Path, "")
	}
	if ref.Opaque != "" {
		dst.User = nil
		dst.Host = ""
		dst.Path = ""
	}
	if ref.Path == "" {
		if ref.RawQuery == "" {
			dst.RawQuery = base.RawQuery
			if ref.Fragment == "" {
				dst.Fragment = base.Fragment
			}
		}
	}
	// The "abs_path" or "rel_path" cases.
	dst.Host = base.Host
	dst.User = base.User
	dst.Path = resolvePath(base.Path, ref.Path)
}

func ParseURL(base *url.URL, ref string, dst *url.URL) error {
	refurl, err := url.Parse(ref)
	if err != nil {
		return err
	}
	ResolveReference(base, refurl, dst)
	return nil
}
