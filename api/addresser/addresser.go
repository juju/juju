// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package addresser

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/watcher"
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

// CanDeallocateAddresses checks if the current model can
// deallocate IP addresses.
func (api *API) CanDeallocateAddresses() (bool, error) {
	var result params.BoolResult
	if err := api.facade.FacadeCall("CanDeallocateAddresses", nil, &result); err != nil {
		return false, errors.Trace(err)
	}
	if result.Error == nil {
		return result.Result, nil
	}
	return false, errors.Trace(result.Error)
}

// CleanupIPAddresses releases and removes the dead IP addresses. If not
// all IP addresses could be released and removed a params.ErrTryAgain
// is returned.
func (api *API) CleanupIPAddresses() error {
	var result params.ErrorResult
	if err := api.facade.FacadeCall("CleanupIPAddresses", nil, &result); err != nil {
		return errors.Trace(err)
	}
	if result.Error == nil {
		return nil
	}
	return errors.Trace(result.Error)
}

var newEntitiesWatcher = apiwatcher.NewEntitiesWatcher

// WatchIPAddresses returns a EntitiesWatcher for observing the
// tags of IP addresses with changes in life cycle.
// The initial event will contain the tags of any IP addresses
// which are no longer Alive.
func (api *API) WatchIPAddresses() (watcher.EntitiesWatcher, error) {
	var result params.EntitiesWatchResult
	err := api.facade.FacadeCall("WatchIPAddresses", nil, &result)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if result.Error == nil {
		w := newEntitiesWatcher(api.facade.RawAPICaller(), result)
		return w, nil
	}
	return nil, errors.Trace(result.Error)
}
