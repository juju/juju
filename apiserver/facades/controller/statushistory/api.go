// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statushistory

import (
	"github.com/juju/juju/v2/apiserver/common"
	apiservererrors "github.com/juju/juju/v2/apiserver/errors"
	"github.com/juju/juju/v2/apiserver/facade"
	"github.com/juju/juju/v2/rpc/params"
	"github.com/juju/juju/v2/state"
)

// API is the concrete implementation of the Pruner endpoint.
type API struct {
	*common.ModelWatcher
	st         *state.State
	authorizer facade.Authorizer
}

// Prune endpoint removes status history entries until
// only the ones newer than now - p.MaxHistoryTime remain and
// the history is smaller than p.MaxHistoryMB.
func (api *API) Prune(p params.StatusHistoryPruneArgs) error {
	if !api.authorizer.AuthController() {
		return apiservererrors.ErrPerm
	}
	return state.PruneStatusHistory(api.st, p.MaxHistoryTime, p.MaxHistoryMB)
}
