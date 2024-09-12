// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statushistory

import (
	"context"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// API is the concrete implementation of the Pruner endpoint.
type API struct {
	st         *state.State
	authorizer facade.Authorizer
}

// Prune performs the status history pruner operation (override for tests).
var Prune = state.PruneStatusHistory

// Prune endpoint removes status history entries until
// only the ones newer than now - p.MaxHistoryTime remain and
// the history is smaller than p.MaxHistoryMB.
func (api *API) Prune(ctx context.Context, p params.StatusHistoryPruneArgs) error {
	if !api.authorizer.AuthController() {
		return apiservererrors.ErrPerm
	}
	return Prune(ctx.Done(), api.st, p.MaxHistoryTime, p.MaxHistoryMB)
}
