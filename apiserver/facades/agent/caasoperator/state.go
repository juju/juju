// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator

import (
	"github.com/juju/juju/state"
	"github.com/juju/juju/status"
)

// CAASOperatorState provides the subset of global state
// required by the CAAS operator facade.
type CAASOperatorState interface {
	Application(string) (Application, error)
}

// Application provides the subets of application state
// requried by the CAAS operator facade.
type Application interface {
	SetStatus(status.StatusInfo) error
}

type stateShim struct {
	*state.State
}

func (s stateShim) Application(id string) (Application, error) {
	return s.State.Application(id)
}
