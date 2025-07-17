// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// ModelMachinesWatcher implements a common WatchModelMachines
// method for use by various facades.
type ModelMachinesWatcher struct {
	machineService  MachineService
	watcherRegistry facade.WatcherRegistry
	authorizer      facade.Authorizer
}

// NewModelMachinesWatcher returns a new ModelMachinesWatcher. The
// GetAuthFunc will be used on each invocation of WatchUnits to
// determine current permissions.
func NewModelMachinesWatcher(machineService MachineService, watcherRegistry facade.WatcherRegistry, authorizer facade.Authorizer) *ModelMachinesWatcher {
	return &ModelMachinesWatcher{
		machineService:  machineService,
		watcherRegistry: watcherRegistry,
		authorizer:      authorizer,
	}
}

// WatchModelMachines returns a StringsWatcher that notifies of
// changes to the life cycles of the top level machines in the current
// model.
func (e *ModelMachinesWatcher) WatchModelMachines(ctx context.Context) (params.StringsWatchResult, error) {
	result := params.StringsWatchResult{}
	if !e.authorizer.AuthController() {
		return result, apiservererrors.ErrPerm
	}

	watcher, err := e.machineService.WatchModelMachines(ctx)
	if err != nil {
		return result, errors.Capture(err)
	}
	watcherId, changes, err := internal.EnsureRegisterWatcher(ctx, e.watcherRegistry, watcher)
	return params.StringsWatchResult{
		Changes:          changes,
		StringsWatcherId: watcherId,
		Error:            apiservererrors.ServerError(err),
	}, nil
}

// WatchModelMachineStartTimes watches the non-container machines in the model
// for changes to the Life or AgentStartTime fields and reports them as a batch.
func (e *ModelMachinesWatcher) WatchModelMachineStartTimes(ctx context.Context) (params.StringsWatchResult, error) {
	if !e.authorizer.AuthController() {
		return params.StringsWatchResult{}, apiservererrors.ErrPerm
	}
	watch, err := e.machineService.WatchModelMachineLifeAndStartTimes(ctx)
	if err != nil {
		return params.StringsWatchResult{}, apiservererrors.ErrPerm
	}

	// Consume the initial event and forward it to the result.
	stringsWatcherId, changes, err := internal.EnsureRegisterWatcher(ctx, e.watcherRegistry, watch)
	return params.StringsWatchResult{
		Changes:          changes,
		StringsWatcherId: stringsWatcherId,
		Error:            apiservererrors.ServerError(err),
	}, nil
}
