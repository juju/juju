// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cleaner

import (
	"context"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

const cleanerFacade = "Cleaner"

// API provides access to the Cleaner API facade.
type API struct {
	facade base.FacadeCaller
}

// NewAPI creates a new client-side Cleaner facade.
func NewAPI(caller base.APICaller) *API {
	facadeCaller := base.NewFacadeCaller(caller, cleanerFacade)
	return &API{facade: facadeCaller}
}

// Cleanup calls the server-side Cleanup method.
func (api *API) Cleanup() error {
	return api.facade.FacadeCall(context.TODO(), "Cleanup", nil, nil)
}

// WatchCleanups calls the server-side WatchCleanups method.
func (api *API) WatchCleanups() (watcher.NotifyWatcher, error) {
	var result params.NotifyWatchResult
	err := api.facade.FacadeCall(context.TODO(), "WatchCleanups", nil, &result)
	if err != nil {
		return nil, err
	}
	if err := result.Error; err != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewNotifyWatcher(api.facade.RawAPICaller(), result)
	return w, nil
}
