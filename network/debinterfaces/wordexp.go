// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package debinterfaces

import "path/filepath"

// WordExpander performs word expansion like a posix-shell.
type WordExpander interface {
	// Expand pattern into a slice of words, or an error if the
	// underlying implementation failed.
	Expand(pattern string) ([]string, error)
}

var _ WordExpander = (*globber)(nil)

type globber struct{}

func newWordExpander() WordExpander {
	return &globber{}
}

func (g *globber) Expand(pattern string) ([]string, error) {
	// The ifupdown package natively used wordexp(3). For the
	// cases we currently need to cater for we can (probably) get
	// away by just globbing. wordexp(3) caters for a lot of
	// shell-style expansions but I haven't seen examples of those
	// in all the ENI files I have seen.
	return filepath.Glob(pattern)
}
