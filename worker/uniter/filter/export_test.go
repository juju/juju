// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package filter

func DummyFilter() Filter {
	// This should, obviously, not be used except for type tests that don't
	// try to do anything with it (eg TestOutput*).
	return &filter{}
}
