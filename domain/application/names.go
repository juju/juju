// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import "regexp"

const (
	// applicationSnippet is a non-compiled regexp that can be composed with
	// other snippets to form a valid application regexp.
	applicationSnippet = "(?:[a-z][a-z0-9]*(?:-[a-z0-9]*[a-z][a-z0-9]*)*)"

	// charmNameSnippet is a non-compiled regexp that can be composed with
	// other snippets to form a valid charm name regexp.
	charmNameSnippet = "[a-z][a-z0-9]*(-[a-z0-9]*[a-z][a-z0-9]*)*"
)

var (
	// validApplication is a regular expression representing application name.
	validApplication = regexp.MustCompile("^" + applicationSnippet + "$")

	// validCharm is a regular expression representing charm name.
	validCharm = regexp.MustCompile("^" + charmNameSnippet + "$")
)

// IsValidApplicationName returns whether name is a valid application name.
func IsValidApplicationName(name string) bool {
	return validApplication.MatchString(name)
}

// IsValidCharmName returns whether name is a valid charm name.
func IsValidCharmName(name string) bool {
	return validCharm.MatchString(name)
}

// IsValidReferenceName returns whether name is a valid reference name.
// This ensures that the reference name is both a valid application name
// and a valid charm name.
func IsValidReferenceName(name string) bool {
	return IsValidApplicationName(name) && IsValidCharmName(name)
}
