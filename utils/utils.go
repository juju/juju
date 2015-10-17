// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

// Must panics if the provided error is not nil.
func Must(err error) {
	if err != nil {
		panic(err)
	}
}
