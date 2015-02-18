// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
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
	err := a.facade.FacadeCall("APIAddresses", nil, &result)
	if err != nil {
		return nil, err
	}

	if err := result.Error; err != nil {
		return nil, err
	}
	return result.Result, nil
}

// EnvironUUID returns the environment UUID to connect to the environment
// that the current connection is for.
func (a *APIAddresser) EnvironUUID() (string, error) {
	var result params.StringResult
	err := a.facade.FacadeCall("EnvironUUID", nil, &result)
	if err != nil {
		return "", err
	}
	return result.Result, nil
}

// CACert returns the certificate used to validate the API and state connections.
func (a *APIAddresser) CACert() (string, error) {
	var result params.BytesResult
	err := a.facade.FacadeCall("CACert", nil, &result)
	if err != nil {
		return "", err
	}
	return string(result.Result), nil
}

// APIHostPorts returns the host/port addresses of the API servers.
func (a *APIAddresser) APIHostPorts() ([][]network.HostPort, error) {
	var result params.APIHostPortsResult
	err := a.facade.FacadeCall("APIHostPorts", nil, &result)
	if err != nil {
		return nil, err
	}
	return result.NetworkHostsPorts(), nil
}

// WatchAPIHostPorts watches the host/port addresses of the API servers.
func (a *APIAddresser) WatchAPIHostPorts() (watcher.NotifyWatcher, error) {
	var result params.NotifyWatchResult
	err := a.facade.FacadeCall("WatchAPIHostPorts", nil, &result)
	if err != nil {
		return nil, err
	}
	return watcher.NewNotifyWatcher(a.facade.RawAPICaller(), result), nil
}
