// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelconfig

import (
	"github.com/juju/names/v6"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/state"
)

// Backend contains the state.State methods used in this package,
// allowing stubs to be created for testing.
type Backend interface {
	ControllerTag() names.ControllerTag
	Sequences() (map[string]int, error)
	SetModelConstraints(value constraints.Value) error
	ModelConstraints() (constraints.Value, error)
}

type stateShim struct {
	*state.State
	model *state.Model
}

// NewStateBackend creates a backend for the facade to use.
func NewStateBackend(m *state.Model) Backend {
	return stateShim{
		State: m.State(),
		model: m,
	}
}
