// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statushistory

import (
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"

	"github.com/juju/loggo"
)

func init() {
	common.RegisterStandardFacade("StatusHistory", 2, NewAPI)
}

var logger = loggo.GetLogger("juju.apiserver.statushistory")

// API is the concrete implementation of the Pruner endpoint..
type API struct {
	st         *state.State
	authorizer common.Authorizer
}

// NewAPI returns an API Instance.
func NewAPI(st *state.State, _ *common.Resources, auth common.Authorizer) (*API, error) {
	return &API{
		st:         st,
		authorizer: auth,
	}, nil
}

// Prune endpoint removes status history entries until
// only the N newest records per unit remain.
func (api *API) Prune(p params.StatusHistoryPruneArgs) error {
	if !api.authorizer.AuthModelManager() {
		return common.ErrPerm
	}
	return state.PruneStatusHistory(api.st, p.MaxLogsPerEntity)
}
