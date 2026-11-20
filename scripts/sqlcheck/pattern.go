// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"path/filepath"
	"strings"
)

// hasGlobMeta reports whether a path argument contains wildcards and
// should be treated as a pattern rather than a literal path.
func hasGlobMeta(arg string) bool {
	return strings.ContainsAny(arg, "*?[")
}

// matchPath reports whether a slash-separated repository path matches a
// slash-separated pattern. A "**" segment matches zero or more whole
// path segments; every other segment follows filepath.Match wildcard
// semantics, which do not cross segment boundaries.
//
// Examples:
//
//	domain/*/state/**   matches domain/cloud/state/state.go and
//	                    domain/cloud/state/testing/cloud.go
//	**/*_test.go        matches every test file
//	domain/secret/**    matches everything under domain/secret
func matchPath(pattern, path string) bool {
	return matchSegments(
		strings.Split(filepath.ToSlash(pattern), "/"),
		strings.Split(filepath.ToSlash(path), "/"),
	)
}

func matchSegments(pat, seg []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {
			// Try consuming zero or more path segments.
			for i := 0; i <= len(seg); i++ {
				if matchSegments(pat[1:], seg[i:]) {
					return true
				}
			}
			return false
		}
		if len(seg) == 0 {
			return false
		}
		ok, err := filepath.Match(pat[0], seg[0])
		if err != nil || !ok {
			return false
		}
		pat, seg = pat[1:], seg[1:]
	}
	return len(seg) == 0
}

// parseRules parses a comma-separated rule list such as "LT01,CP01"
// into a set. An empty list yields a nil set meaning "no filtering".
func parseRules(spec string) map[string]bool {
	if strings.TrimSpace(spec) == "" {
		return nil
	}
	rules := map[string]bool{}
	for r := range strings.SplitSeq(spec, ",") {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		rules[strings.ToUpper(r)] = true
	}
	if len(rules) == 0 {
		return nil
	}
	return rules
}
