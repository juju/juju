// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// APIAddresser provides common client-side API
// functions to call into apiserver.common.APIAddresser
type APIAddresser struct {
	facade base.FacadeCaller
}

// NewAPIAddresser returns a new APIAddresser that makes API calls
// using caller and the specified facade name.
func NewAPIAddresser(facade base.FacadeCaller) *APIAddresser {
	return &APIAddresser{
		facade: facade,
	}
}

// APIAddresses returns the list of addresses used to connect to the API.
func (a *APIAddresser) APIAddresses() ([]string, error) {
	var result params.StringsResult
	err := a.facade.FacadeCall(context.TODO(), "APIAddresses", nil, &result)
	if err != nil {
		return nil, err
	}

	if err := result.Error; err != nil {
		return nil, err
	}
	return result.Result, nil
}

// APIHostPorts returns the host/port addresses of the API servers.
func (a *APIAddresser) APIHostPorts() ([]network.ProviderHostPorts, error) {
	var result params.APIHostPortsResult
	err := a.facade.FacadeCall(context.TODO(), "APIHostPorts", nil, &result)
	if err != nil {
		return nil, err
	}
	return params.ToProviderHostsPorts(result.Servers), nil
}

// WatchAPIHostPorts watches the host/port addresses of the API servers.
func (a *APIAddresser) WatchAPIHostPorts() (watcher.NotifyWatcher, error) {
	var result params.NotifyWatchResult
	err := a.facade.FacadeCall(context.TODO(), "WatchAPIHostPorts", nil, &result)
	if err != nil {
		return nil, err
	}
	return apiwatcher.NewNotifyWatcher(a.facade.RawAPICaller(), result), nil
}
