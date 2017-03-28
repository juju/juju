// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"github.com/juju/juju/state"
)

// Backend provides selected methods off the state.State struct.
type Backend interface {
	Application(name string) (*state.Application, error)
}

var getStateAccess = func(st *state.State) Backend {
	return &stateShim{st}
}

type stateShim struct {
	*state.State
}
