// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statushistory

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

// API is the concrete implementation of the Pruner endpoint.
type API struct {
	*common.ModelWatcher
	cancel     <-chan struct{}
	st         *state.State
	authorizer facade.Authorizer
}

// NewAPI returns an API Instance.
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

// Prune performs the status history pruner operation (override for tests).
var Prune = state.PruneStatusHistory

// Prune endpoint removes status history entries until
// only the ones newer than now - p.MaxHistoryTime remain and
// the history is smaller than p.MaxHistoryMB.
func (api *API) Prune(p params.StatusHistoryPruneArgs) error {
	if !api.authorizer.AuthController() {
		return apiservererrors.ErrPerm
	}
	return Prune(api.st, p.MaxHistoryTime, p.MaxHistoryMB)
}
