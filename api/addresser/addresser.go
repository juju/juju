// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package addresser

import (
	"github.com/juju/loggo"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
)

var logger = loggo.GetLogger("juju.api.addresser")

const addresserFacade = "Addresser"

// API provides access to the InstancePoller API facade.
type API struct {
	facade base.FacadeCaller
}

// NewAPI creates a new client-side Addresser facade.
func NewAPI(caller base.APICaller) *API {
	if caller == nil {
		panic("caller is nil")
	}
	return &API{
		facade: base.NewFacadeCaller(caller, addresserFacade),
	}
}

// CleanupIPAddresses releases and removes the dead IP addresses.
func (api *API) CleanupIPAddresses() error {
	var result params.ErrorResult
	// If not all IP addresses could be released and removed a params.ErrTryAgain
	// is returned.
	if err := api.facade.FacadeCall("CleanupIPAddresses", nil, &result); err != nil {
		return err
	}
	if result.Error != nil {
		return result.Error
	}
	return nil
}

var newEntityWatcher = watcher.NewEntityWatcher

// WatchIPAddresses returns a EntityWatcher for observing the
// tags of IP addresses with changes in life cycle.
// The initial event will contain the tags of any IP addresses
// which are no longer Alive.
func (api *API) WatchIPAddresses() (watcher.EntityWatcher, error) {
	var result params.EntityWatchResult
	err := api.facade.FacadeCall("WatchIPAddresses", nil, &result)
	if err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	if result.Error != nil {
		return nil, result.Error
	}
	w := newEntityWatcher(api.facade.RawAPICaller(), result)
	return w, nil
}
