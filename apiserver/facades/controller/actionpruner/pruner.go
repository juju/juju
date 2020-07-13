// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package actionpruner

import (
	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

type API struct {
	*common.ModelWatcher
	st         *state.State
	model      *state.Model
	authorizer facade.Authorizer
}

func NewAPI(st *state.State, r facade.Resources, auth facade.Authorizer) (*API, error) {
	m, err := st.Model()
	if err != nil {
		return nil, err
	}

	return &API{
		ModelWatcher: common.NewModelWatcher(m, r, auth),
		st:           st,
		authorizer:   auth,
	}, nil
}

func (api *API) Prune(p params.ActionPruneArgs) error {
	if !api.authorizer.AuthController() {
		return apiservererrors.ErrPerm
	}

	return state.PruneOperations(api.st, p.MaxHistoryTime, p.MaxHistoryMB)
}
