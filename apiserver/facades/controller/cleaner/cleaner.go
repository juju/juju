// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The cleaner package implements the API interface
// used by the cleaner worker.

package cleaner

import (
	apiservererrors "github.com/juju/juju/v3/apiserver/errors"
	"github.com/juju/juju/v3/apiserver/facade"
	"github.com/juju/juju/v3/rpc/params"
	"github.com/juju/juju/v3/state/watcher"
)

// CleanerAPI implements the API used by the cleaner worker.
type CleanerAPI struct {
	st        StateInterface
	resources facade.Resources
}

// Cleanup triggers a state cleanup
func (api *CleanerAPI) Cleanup() error {
	return api.st.Cleanup()
}

// WatchCleanups watches for cleanups to be performed in state.
func (api *CleanerAPI) WatchCleanups() (params.NotifyWatchResult, error) {
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
