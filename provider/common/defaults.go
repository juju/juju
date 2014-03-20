// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/state"
)

// DoesSupportUnitPlacement provides an
// implementation of SupportsUnitPlacement
// that never returns an error, and is
// intended for embedding in environs.Environ
// implementations.
type DoesSupportUnitPlacement struct{}

func (*DoesSupportUnitPlacement) SupportsUnitPlacement() error {
	return nil
}

// NopPrechecker provides an implementation of the
// state.Prechecker interface that passes all checks.
type NopPrechecker struct{}

var _ state.Prechecker = (*NopPrechecker)(nil)

func (*NopPrechecker) PrecheckInstance(series string, cons constraints.Value) error {
	return nil
}
