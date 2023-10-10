// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodelconfigmanager

import (
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/context_mock.go github.com/juju/juju/apiserver/facade Authorizer,Context,Resources

// State provides required state for the Facade.
type State interface {
	WatchControllerConfig() state.NotifyWatcher
}

// Facade allows model config manager clients to watch controller config changes and fetch controller config.
type Facade struct {
	auth      facade.Authorizer
	resources facade.Resources

	ctrlState           State
	controllerConfigAPI *common.ControllerConfigAPI
}

func (f *Facade) ControllerConfig() (params.ControllerConfigResult, error) {
	return f.controllerConfigAPI.ControllerConfig()
}

func (f *Facade) WatchControllerConfig() (params.NotifyWatchResult, error) {
	result := params.NotifyWatchResult{}
	w := f.ctrlState.WatchControllerConfig()
	if _, ok := <-w.Changes(); ok {
		result.NotifyWatcherId = f.resources.Register(w)
	} else {
		return result, watcher.EnsureErr(w)
	}
	return result, nil
}
