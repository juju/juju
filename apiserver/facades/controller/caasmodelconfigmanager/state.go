// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodelconfigmanager

import (
	jujucontroller "github.com/juju/juju/controller"
	"github.com/juju/juju/state"
)

type Backend interface {
	WatchControllerConfig() state.NotifyWatcher
}

// ControllerState provides the subset of controller state
// required by the CAAS application facade.
type ControllerState interface {
	ControllerConfig() (jujucontroller.Config, error)
}

type stateShim struct {
	st *state.State
}

func (shim stateShim) WatchControllerConfig() state.NotifyWatcher {
	return shim.st.WatchControllerConfig()
}

var getState = func(st *state.State) Backend {
	return stateShim{st}
}
