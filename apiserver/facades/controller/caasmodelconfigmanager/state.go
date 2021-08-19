// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodelconfigmanager

import (
	jujucontroller "github.com/juju/juju/controller"
	"github.com/juju/juju/state"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/state_mock.go github.com/juju/juju/apiserver/facades/controller/caasmodelconfigmanager Backend,ControllerState
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/watcher_mock.go github.com/juju/juju/state NotifyWatcher

type Backend interface {
	WatchControllerConfig() state.NotifyWatcher
}

// ControllerState provides the subset of controller state
// required by the CAAS application facade.
type ControllerState interface {
	ControllerConfig() (jujucontroller.Config, error)
}

type stateShim struct {
	st Backend
}

func (shim stateShim) WatchControllerConfig() state.NotifyWatcher {
	return shim.st.WatchControllerConfig()
}

var getState = func(st Backend) Backend {
	return stateShim{st}
}
