// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/juju/network"
	"github.com/juju/juju/state/api/base"
	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/state/api/watcher"
)

// APIAddresser provides common client-side API
// functions to call into apiserver.common.APIAddresser
type APIAddresser struct {
	facadeName string
	caller     base.Caller
}

// NewAPIAddresser returns a new APIAddresser that makes API calls
// using caller and the specified facade name.
func NewAPIAddresser(facadeName string, caller base.Caller) *APIAddresser {
	return &APIAddresser{
		facadeName: facadeName,
		caller:     caller,
	}
}

// APIAddresses returns the list of addresses used to connect to the API.
func (a *APIAddresser) APIAddresses() ([]string, error) {
	var result params.StringsResult
	err := a.caller.Call(a.facadeName, 0, "", "APIAddresses", nil, &result)
	if err != nil {
		return nil, err
	}

	if err := result.Error; err != nil {
		return nil, err
	}
	return result.Result, nil
}

// CACert returns the certificate used to validate the API and state connections.
func (a *APIAddresser) CACert() (string, error) {
	var result params.BytesResult
	err := a.caller.Call(a.facadeName, 0, "", "CACert", nil, &result)
	if err != nil {
		return "", err
	}
	return string(result.Result), nil
}

// APIHostPorts returns the host/port addresses of the API servers.
func (a *APIAddresser) APIHostPorts() ([][]network.HostPort, error) {
	var result params.APIHostPortsResult
	err := a.caller.Call(a.facadeName, 0, "", "APIHostPorts", nil, &result)
	if err != nil {
		return nil, err
	}
	return result.Servers, nil
}

// WatchAPIHostPorts watches the host/port addresses of the API servers.
func (a *APIAddresser) WatchAPIHostPorts() (watcher.NotifyWatcher, error) {
	var result params.NotifyWatchResult
	err := a.caller.Call(a.facadeName, 0, "", "WatchAPIHostPorts", nil, &result)
	if err != nil {
		return nil, err
	}
	return watcher.NewNotifyWatcher(a.caller, result), nil
}
