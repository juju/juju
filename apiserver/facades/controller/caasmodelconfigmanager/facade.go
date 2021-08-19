// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodelconfigmanager

import (
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state/watcher"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/context_mock.go github.com/juju/juju/apiserver/facade Authorizer,Context,Resources

// Facade allows model config manager clients to watch controller config changes and fetch controller config.
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
	return newFacade(
		getState(st),
		ctx.Resources(),
		ctx.StatePool().SystemState(),
	)
}

func newFacade(
	backend Backend,
	resources facade.Resources,
	ctrlSt ControllerState,
) (*Facade, error) {
	return &Facade{
		backend:   backend,
		resources: resources,
		ctrlSt:    ctrlSt,
	}, nil
}

// Watch returns a watcher that notifies controller config changes.
func (f *Facade) WatchControllerConfig() (params.NotifyWatchResult, error) {
	watch := f.backend.WatchControllerConfig()
	if _, ok := <-watch.Changes(); ok {
		return params.NotifyWatchResult{
			NotifyWatcherId: f.resources.Register(watch),
		}, nil
	}
	return params.NotifyWatchResult{
		Error: apiservererrors.ServerError(watcher.EnsureErr(watch)),
	}, nil
}

// ControllerConfig returns the controller's configuration.
func (f *Facade) ControllerConfig() (params.ControllerConfigResult, error) {
	result := params.ControllerConfigResult{}
	config, err := f.ctrlSt.ControllerConfig()
	if err != nil {
		return result, err
	}
	result.Config = params.ControllerConfig(config)
	return result, nil
}
