// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config

const (
	// JujuSystemKey is the SSH key comment for Juju system keys.
	JujuSystemKey = "juju-system-key"
)

// ConcatAuthKeys concatenates the two sets of authorised keys, interposing
// a newline if necessary, because authorised keys are newline-separated.
func ConcatAuthKeys(a, b string) string {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	if a[len(a)-1] != '\n' {
		return a + "\n" + b
	}
	return a + b
}
