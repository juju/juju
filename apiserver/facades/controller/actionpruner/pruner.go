// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package actionpruner

import (
	"context"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// API provides access to the action pruner API.
type API struct {
	st         *state.State
	authorizer facade.Authorizer
}

// Prune performs the action pruner operation (override for tests).
var Prune = state.PruneOperations

// Prune endpoint removes action entries until
// only the ones newer than now - p.MaxHistoryTime remain and
// the history is smaller than p.MaxHistoryMB.
func (api *API) Prune(ctx context.Context, p params.ActionPruneArgs) error {
	if !api.authorizer.AuthController() {
		return apiservererrors.ErrPerm
	}

	return Prune(ctx.Done(), api.st, p.MaxHistoryTime, p.MaxHistoryMB)
}
