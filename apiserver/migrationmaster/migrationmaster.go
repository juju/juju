// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

func init() {
	common.RegisterStandardFacade("MigrationMaster", 1, NewAPI)
}

// API implements the API required for the model migration
// master worker.
type API struct {
	state      Backend
	authorizer common.Authorizer
	resources  *common.Resources
}

// NewAPI creates a new API server endpoint for the model migration
// master worker.
func NewAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*API, error) {
	if !authorizer.AuthModelManager() {
		return nil, common.ErrPerm
	}
	return &API{
		state:      getBackend(st),
		authorizer: authorizer,
		resources:  resources,
	}, nil
}

// Watch starts watching for an active migration for the model
// associated with the API connection. The returned id should be used
// with Next on the MigrationMasterWatcher endpoint to receive deltas.
func (api *API) Watch() (params.NotifyWatchResult, error) {
	w, err := api.state.WatchForModelMigration()
	if err != nil {
		return params.NotifyWatchResult{}, errors.Trace(err)
	}
	return params.NotifyWatchResult{
		NotifyWatcherId: api.resources.Register(w),
	}, nil
}
