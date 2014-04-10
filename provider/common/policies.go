// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/state"
)

// SupportsUnitPlacementPolicy provides an
// implementation of SupportsUnitPlacement
// that never returns an error, and is
// intended for embedding in environs.Environ
// implementations.
type SupportsUnitPlacementPolicy struct{}

func (*SupportsUnitPlacementPolicy) SupportsUnitPlacement() error {
	return nil
}

// NopPrecheckerPolicy provides an implementation of the
// state.Prechecker interface that passes all checks.
type NopPrecheckerPolicy struct{}

var _ state.Prechecker = (*NopPrecheckerPolicy)(nil)

func (*NopPrecheckerPolicy) PrecheckInstance(series string, cons constraints.Value) error {
	return nil
}
