// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

// Returns a ptr for the supplied 32 bit integer
func Int32Ptr(i int32) *int32 {
	return &i
}
