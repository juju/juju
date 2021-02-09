// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logpruner

import (
	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

// API is the log pruner API.
type API struct {
	*common.ControllerConfigAPI
	cancel       <-chan struct{}
	controllerSt *state.State
	model        *state.Model
	authorizer   facade.Authorizer
}

// NewAPI makes a new log pruner API.
func NewAPI(ctx facade.Context) (*API, error) {
	return &API{
		ControllerConfigAPI: common.NewStateControllerConfig(ctx.State(), ctx.Resources()),
		controllerSt:        SystemState(ctx),
		authorizer:          ctx.Auth(),
		cancel:              ctx.Cancel(),
	}, nil
}

// SystemState returns the system state for a context (override for tests).
var SystemState = func(ctx facade.Context) *state.State {
	return ctx.StatePool().SystemState()
}

// Prune performs the log pruner operation (override for tests).
var Prune = state.PruneLogs

// Prune prunes the logs collection.
func (api *API) Prune(p params.LogPruneArgs) error {
	if !api.authorizer.AuthController() {
		return apiservererrors.ErrPerm
	}

	return Prune(api.cancel, api.controllerSt, p.MaxLogMB)
}
