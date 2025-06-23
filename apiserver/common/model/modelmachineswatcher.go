// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"
	"time"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// watchMachinesQuiesceInterval specifies the time window for batching together
// changes to life and agent start times when watching the machine collection
// for a particular model. For more information, see the WatchModelMachineStartTimes
// method in state/watcher.go.
const watchMachinesQuiesceInterval = 500 * time.Millisecond

// ModelMachinesWatcher implements a common WatchModelMachines
// method for use by various facades.
type ModelMachinesWatcher struct {
	st              state.ModelMachinesWatcher
	machineService  MachineService
	watcherRegistry facade.WatcherRegistry
	authorizer      facade.Authorizer
}

// NewModelMachinesWatcher returns a new ModelMachinesWatcher. The
// GetAuthFunc will be used on each invocation of WatchUnits to
// determine current permissions.
func NewModelMachinesWatcher(st state.ModelMachinesWatcher, machineService MachineService, watcherRegistry facade.WatcherRegistry, authorizer facade.Authorizer) *ModelMachinesWatcher {
	return &ModelMachinesWatcher{
		st:              st,
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
	result.StringsWatcherId, _, err = internal.EnsureRegisterWatcher[[]string](ctx, e.watcherRegistry, watcher)
	return result, err
}

// WatchModelMachineStartTimes watches the non-container machines in the model
// for changes to the Life or AgentStartTime fields and reports them as a batch.
func (e *ModelMachinesWatcher) WatchModelMachineStartTimes(ctx context.Context) (params.StringsWatchResult, error) {
	result := params.StringsWatchResult{}
	if !e.authorizer.AuthController() {
		return result, apiservererrors.ErrPerm
	}
	watch := e.st.WatchModelMachineStartTimes(watchMachinesQuiesceInterval)
	// Consume the initial event and forward it to the result.
	stringsWatcherId, changes, err := internal.EnsureRegisterWatcher[[]string](ctx, e.watcherRegistry, watch)
	result.Changes = changes
	result.StringsWatcherId = stringsWatcherId
	return result, err
}
