// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/state"
)

// EnvironBase provides default implementations of
// some environs.Environ interface methods, and may
// be embedded within an Environ implementation.
type EnvironBase struct{}

var _ state.Prechecker = (*EnvironBase)(nil)

func (*EnvironBase) SupportsUnitPlacement() error {
	return nil // supported by default
}

func (*EnvironBase) PrecheckInstance(series string, cons constraints.Value) error {
	return nil // all instances allowed by default
}
