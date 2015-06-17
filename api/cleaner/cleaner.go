// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cleaner

import (
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
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
	return api.facade.FacadeCall("Cleanup", nil, nil)
}

// WatchCleanups calls the server-side WatchCleanups method.
func (api *API) WatchCleanups() (watcher.NotifyWatcher, error) {
	var result params.NotifyWatchResult
	err := api.facade.FacadeCall("WatchCleanups", nil, &result)
	if err != nil {
		return nil, err
	}
	if err := result.Error; err != nil {
		return nil, result.Error
	}
	w := watcher.NewNotifyWatcher(api.facade.RawAPICaller(), result)
	return w, nil
}
