// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The cleaner package implements the API interface
// used by the cleaner worker.

package cleaner

import (
	"github.com/juju/loggo"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

func init() {
	common.RegisterStandardFacade("Cleaner", 1, NewCleanerAPI)
}

var logger = loggo.GetLogger("juju.apiserver.cleaner")

// CleanerAPI implements the API used by the cleaner worker.
type CleanerAPI struct {
	st        StateInterface
	resources *common.Resources
}

// NewCleanerAPI creates a new instance of the Cleaner API.
func NewCleanerAPI(
	st *state.State,
	res *common.Resources,
	authorizer common.Authorizer,
) (*CleanerAPI, error) {
	if !authorizer.AuthEnvironManager() {
		return nil, common.ErrPerm
	}
	return &CleanerAPI{
		st:        getState(st),
		resources: res,
	}, nil
}

// Cleanup triggers a state cleanup
func (api *CleanerAPI) Cleanup() error {
	return api.st.Cleanup()
}

// WatchChanges watches for cleanups to be perfomed in state
func (api *CleanerAPI) WatchCleanups() (params.NotifyWatchResult, error) {
	watch := api.st.WatchCleanups()
	if _, ok := <-watch.Changes(); ok {
		return params.NotifyWatchResult{
			NotifyWatcherId: api.resources.Register(watch),
		}, nil
	}
	return params.NotifyWatchResult{
		Error: common.ServerError(watcher.EnsureErr(watch)),
	}, nil
}
