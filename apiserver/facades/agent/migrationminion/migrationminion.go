// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationminion

import (
	"context"

	"github.com/juju/errors"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/rpc/params"
)

// API implements the API required for the model migration
// master worker.
type API struct {
	backend    Backend
	authorizer facade.Authorizer
	resources  facade.Resources
}

// NewAPI creates a new API server endpoint for the model migration
// master worker.
func NewAPI(
	backend Backend,
	resources facade.Resources,
	authorizer facade.Authorizer,
) (*API, error) {
	if !(authorizer.AuthMachineAgent() || authorizer.AuthUnitAgent() || authorizer.AuthApplicationAgent()) {
		return nil, apiservererrors.ErrPerm
	}
	return &API{
		backend:    backend,
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
func (api *API) Watch(ctx context.Context) (params.NotifyWatchResult, error) {
	w := api.backend.WatchMigrationStatus()
	return params.NotifyWatchResult{
		NotifyWatcherId: api.resources.Register(w),
	}, nil
}

// Report allows a migration minion to submit whether it succeeded or
// failed for a specific migration phase.
func (api *API) Report(ctx context.Context, info params.MinionReport) error {
	phase, ok := migration.ParsePhase(info.Phase)
	if !ok {
		return errors.New("unable to parse phase")
	}

	mig, err := api.backend.Migration(info.MigrationId)
	if err != nil {
		return errors.Trace(err)
	}

	err = mig.SubmitMinionReport(api.authorizer.GetAuthTag(), phase, info.Success)
	return errors.Trace(err)
}
