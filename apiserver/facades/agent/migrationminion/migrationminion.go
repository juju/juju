// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationminion

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/rpc/params"
)

// API implements the API required for the model migration
// master worker.
type API struct {
	watcherRegistry       facade.WatcherRegistry
	authorizer            facade.Authorizer
	modelMigrationService ModelMigrationService
}

// NewAPI creates a new API server endpoint for the model migration
// master worker.
func NewAPI(
	watcherRegistry facade.WatcherRegistry,
	authorizer facade.Authorizer,
	modelMigrationService ModelMigrationService,
) (*API, error) {
	if !(authorizer.AuthMachineAgent() || authorizer.AuthUnitAgent()) {
		return nil, apiservererrors.ErrPerm
	}
	return &API{
		watcherRegistry:       watcherRegistry,
		authorizer:            authorizer,
		modelMigrationService: modelMigrationService,
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
	var res params.NotifyWatchResult
	w, err := api.modelMigrationService.WatchForMigration(ctx)
	if err != nil {
		res.Error = apiservererrors.ServerError(err)
		return res, nil
	}
	// Do not pull initial results. This is consumed as a legacy watcher called
	// MigrationStatusWatcher.
	res.NotifyWatcherId, err = api.watcherRegistry.Register(ctx, w)
	if err != nil {
		res.Error = apiservererrors.ServerError(err)
		w.Kill()
		return res, nil
	}
	return res, nil
}

// Report allows a migration minion to submit whether it succeeded or
// failed for a specific migration phase.
func (api *API) Report(ctx context.Context, info params.MinionReport) error {
	phase, ok := migration.ParsePhase(info.Phase)
	if !ok {
		return errors.New("unable to parse phase")
	}

	tag := api.authorizer.GetAuthTag()
	switch t := tag.(type) {
	case names.UnitTag:
		return api.modelMigrationService.ReportFromUnit(
			ctx, unit.Name(t.Id()), phase)
	case names.MachineTag:
		return api.modelMigrationService.ReportFromMachine(
			ctx, machine.Name(t.Id()), phase)
	default:
		return errors.NotSupportedf("reporting minion status for %v", tag)
	}
}
