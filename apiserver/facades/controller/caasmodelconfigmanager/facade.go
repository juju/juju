// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodelconfigmanager

import (
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state/watcher"
)

// Facade allows model config manager clients to watch config changes.
type Facade struct {
	backend   Backend
	resources facade.Resources
	ctrlSt    ControllerState
}

// NewFacade creates a new authorized Facade.
func NewFacade(ctx facade.Context) (*Facade, error) {
	if !ctx.Auth().AuthController() {
		return nil, apiservererrors.ErrPerm
	}
	st := ctx.State()
	return &Facade{
		backend:   getState(st),
		resources: ctx.Resources(),
		ctrlSt:    ctx.StatePool().SystemState(),
	}, nil
}

// Watch returns a watcher that sends the names of services whose
// unit count may be below their configured minimum.
func (facade *Facade) WatchControllerConfig() (params.NotifyWatchResult, error) {
	watch := facade.backend.WatchControllerConfig()
	if _, ok := <-watch.Changes(); ok {
		return params.NotifyWatchResult{
			NotifyWatcherId: facade.resources.Register(watch),
		}, nil
	}
	return params.NotifyWatchResult{
		Error: apiservererrors.ServerError(watcher.EnsureErr(watch)),
	}, nil
}

// ControllerConfig returns the controller's configuration.
func (facade *Facade) ControllerConfig() (params.ControllerConfigResult, error) {
	result := params.ControllerConfigResult{}
	config, err := facade.ctrlSt.ControllerConfig()
	if err != nil {
		return result, err
	}
	result.Config = params.ControllerConfig(config)
	return result, nil
}
