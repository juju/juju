// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The cleaner package implements the API interface
// used by the cleaner worker.

package cleaner

import (
	"context"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

// CleanerAPI implements the API used by the cleaner worker.
type CleanerAPI struct {
	st             StateInterface
	resources      facade.Resources
	objectStore    objectstore.ObjectStore
	machineRemover state.MachineRemover
}

// Cleanup triggers a state cleanup
func (api *CleanerAPI) Cleanup(ctx context.Context) error {
	return api.st.Cleanup(ctx, api.objectStore, api.machineRemover)
}

// WatchCleanups watches for cleanups to be performed in state.
func (api *CleanerAPI) WatchCleanups(ctx context.Context) (params.NotifyWatchResult, error) {
	watch := api.st.WatchCleanups()
	if _, ok := <-watch.Changes(); ok {
		return params.NotifyWatchResult{
			NotifyWatcherId: api.resources.Register(watch),
		}, nil
	}
	return params.NotifyWatchResult{
		Error: apiservererrors.ServerError(watcher.EnsureErr(watch)),
	}, nil
}
