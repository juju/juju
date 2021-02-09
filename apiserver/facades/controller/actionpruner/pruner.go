// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package actionpruner

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

// API provides access to the action pruner API.
type API struct {
	*common.ModelWatcher
	cancel     <-chan struct{}
	st         *state.State
	model      *state.Model
	authorizer facade.Authorizer
}

// NewAPI returns an action pruner API.
func NewAPI(ctx facade.Context) (*API, error) {
	m, err := Model(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &API{
		ModelWatcher: common.NewModelWatcher(m, ctx.Resources(), ctx.Auth()),
		st:           ctx.State(),
		authorizer:   ctx.Auth(),
		cancel:       ctx.Cancel(),
	}, nil
}

// Model returns the model for a context (override for tests).
var Model = func(ctx facade.Context) (state.ModelAccessor, error) {
	return ctx.State().Model()
}

// Prune performs the action pruner operation (override for tests).
var Prune = state.PruneOperations

// Prune endpoint removes action entries until
// only the ones newer than now - p.MaxHistoryTime remain and
// the history is smaller than p.MaxHistoryMB.
func (api *API) Prune(p params.ActionPruneArgs) error {
	if !api.authorizer.AuthController() {
		return apiservererrors.ErrPerm
	}

	return Prune(api.cancel, api.st, p.MaxHistoryTime, p.MaxHistoryMB)
}
