// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	coremigration "github.com/juju/juju/core/migration"
	"github.com/juju/juju/state"
)

func init() {
	common.RegisterStandardFacade("MigrationMaster", 1, NewAPI)
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
	if !authorizer.AuthModelManager() {
		return nil, common.ErrPerm
	}
	return &API{
		backend:    getBackend(st),
		authorizer: authorizer,
		resources:  resources,
	}, nil
}

// Watch starts watching for an active migration for the model
// associated with the API connection. The returned id should be used
// with Next on the MigrationMasterWatcher endpoint to receive deltas.
func (api *API) Watch() (params.NotifyWatchResult, error) {
	w, err := api.backend.WatchForModelMigration()
	if err != nil {
		return params.NotifyWatchResult{}, errors.Trace(err)
	}
	return params.NotifyWatchResult{
		NotifyWatcherId: api.resources.Register(w),
	}, nil
}

// SetPhase sets the phase of the active model migration. The provided
// phase must be a valid phase value, for example QUIESCE" or
// "ABORT". See the core/migration package for the complete list.
func (api *API) SetPhase(args params.SetMigrationPhaseArgs) error {
	mig, err := api.backend.GetModelMigration()
	if err != nil {
		return errors.Annotate(err, "could not get migration")
	}

	phase, ok := coremigration.ParsePhase(args.Phase)
	if !ok {
		return errors.Errorf("invalid phase: %q", args.Phase)
	}

	err = mig.SetPhase(phase)
	if err != nil {
		return errors.Annotate(err, "failed to set phase")
	}
	return nil
}

// Export serializes the model associated with the API connection.
func (api *API) Export() (params.SerializedModel, error) {
	var serialized params.SerializedModel

	bytes, err := exportModel(api.backend)
	if err != nil {
		return serialized, err
	}

	serialized.Bytes = bytes
	return serialized, nil
}
