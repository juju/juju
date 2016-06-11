// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statushistory

import (
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

func init() {
	common.RegisterStandardFacade("StatusHistory", 2, NewAPI)
}

// API is the concrete implementation of the Pruner endpoint..
type API struct {
	st         *state.State
	authorizer facade.Authorizer
}

// NewAPI returns an API Instance.
func NewAPI(st *state.State, _ facade.Resources, auth facade.Authorizer) (*API, error) {
	return &API{
		st:         st,
		authorizer: auth,
	}, nil
}

// Prune endpoint removes status history entries until
// only the ones newer than now - p.MaxHistoryTime remain and
// the history is smaller than p.MaxHistoryMB.
func (api *API) Prune(p params.StatusHistoryPruneArgs) error {
	if !api.authorizer.AuthModelManager() {
		return common.ErrPerm
	}
	return state.PruneStatusHistory(api.st, p.MaxHistoryTime, p.MaxHistoryMB)
}
