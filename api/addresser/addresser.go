// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package addresser

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"

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

// IPAddresses retrieves the IP addresses with the given tags.
func (api *API) IPAddresses(tags ...names.IPAddressTag) ([]*IPAddress, error) {
	var results params.LifeResults
	args := params.Entities{
		Entities: make([]params.Entity, len(tags)),
	}
	for i, tag := range tags {
		args.Entities[i].Tag = tag.String()
	}
	if err := api.facade.FacadeCall("Life", args, &results); err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != len(tags) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(tags), len(results.Results))
	}
	var err error
	ipAddresses := make([]*IPAddress, len(tags))
	for i, result := range results.Results {
		if result.Error != nil {
			logger.Warningf("error retieving IP address %v: %v", tags[i], result.Error)
			ipAddresses[i] = nil
			err = common.ErrPartialResults
		} else {
			ipAddresses[i] = &IPAddress{api.facade, tags[i], result.Life}
		}
	}
	return ipAddresses, err
}

// Remove deletes the given IP addresses.
func (api *API) Remove(ipAddresses ...*IPAddress) error {
	var results params.ErrorResults
	args := params.Entities{
		Entities: make([]params.Entity, len(ipAddresses)),
	}
	for i, ipAddress := range ipAddresses {
		args.Entities[i].Tag = ipAddress.Tag().String()
	}
	if err := api.facade.FacadeCall("Remove", args, &results); err != nil {
		return errors.Trace(err)
	}
	return results.Combine()
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
