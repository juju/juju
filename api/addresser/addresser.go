// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package addresser

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
)

var logger = loggo.GetLogger("juju.api.addresser")

const addresserFacade = "Addresser"

// API provides access to the InstancePoller API facade.
type API struct {
	*common.EnvironWatcher

	facade base.FacadeCaller
}

// NewAPI creates a new client-side Addresser facade.
func NewAPI(caller base.APICaller) *API {
	if caller == nil {
		panic("caller is nil")
	}
	facadeCaller := base.NewFacadeCaller(caller, addresserFacade)
	return &API{
		EnvironWatcher: common.NewEnvironWatcher(facadeCaller),
		facade:         facadeCaller,
	}
}

// CleanupIPAddresses releases and removes the dead IP addresses.
func (api *API) CleanupIPAddresses() error {
	if err := api.facade.FacadeCall("CleanupIPAddresses", nil, nil); err != nil {
		// TODO(mue) Does tracing make sense here?
		return errors.Trace(err)
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
