// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelconfig

// Backend contains the state.State methods used in this package,
// allowing stubs to be created for testing.
type Backend interface {
	Sequences() (map[string]int, error)
}
