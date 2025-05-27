// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

// Returns a ptr for the supplied integer.
func IntPtr(i int) *int {
	return &i
}
