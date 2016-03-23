// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationminion

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

func init() {
	common.RegisterStandardFacade("MigrationMinion", 1, NewAPI)
}

// API implements the API required for the model migration
// master worker.
type API struct {
	backend    Backend
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
	if !(authorizer.AuthMachineAgent() || authorizer.AuthUnitAgent()) {
		return nil, common.ErrPerm
	}
	return &API{
		backend:    getBackend(st),
		authorizer: authorizer,
		resources:  resources,
	}, nil
}

// Watch starts watching for status updates for a migration attempt
// for the model. It will report when a migration starts and when its
// status changes (including when it finishes). An initial event will
// be fired if there has ever been a migration attempt for the model.
//
// The MigrationStatusWatcher facade must be used to receive events
// from the watcher.
func (api *API) Watch() (params.NotifyWatchResult, error) {
	w, err := api.backend.WatchMigrationStatus()
	if err != nil {
		return params.NotifyWatchResult{}, errors.Trace(err)
	}
	return params.NotifyWatchResult{
		NotifyWatcherId: api.resources.Register(w),
	}, nil
}
