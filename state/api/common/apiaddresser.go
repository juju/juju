// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state/api/base"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/api/watcher"
)

type APIAddresser struct {
	facadeName string
	caller     base.Caller
}

// NewAPIAddresser returns
func NewAPIAddresser(facadeName string, caller base.Caller) *APIAddresser {
	return &APIAddresser{
		facadeName: facadeName,
		caller:     caller,
	}
}

// APIAddresses returns the list of addresses used to connect to the API.
func (a *APIAddresser) APIAddresses() ([]string, error) {
	var result params.StringsResult
	err := a.caller.Call(a.facadeName, "", "APIAddresses", nil, &result)
	if err != nil {
		return nil, err
	}

	if err := result.Error; err != nil {
		return nil, err
	}
	return result.Result, nil
}

// CACert returns the certificate used to validate the state connection.
func (a *APIAddresser) CACert() ([]byte, error) {
	var result params.BytesResult
	err := a.caller.Call(a.facadeName, "", "CACert", nil, &result)
	if err != nil {
		return nil, err
	}
	return result.Result, nil
}

// APIHostPorts returns the host/port addresses of the API servers.
func (a *APIAddresser) APIHostPorts() ([][]instance.HostPort, error) {
	var result params.APIHostPortsResult
	err := a.caller.Call(a.facadeName, "", "APIHostPorts", nil, &result)
	if err != nil {
		return nil, err
	}
	return result.Servers, nil
}

// APIHostPorts watches the host/port addresses of the API servers.
func (a *APIAddresser) WatchAPIHostPorts() (watcher.NotifyWatcher, error) {
	var result params.NotifyWatchResult
	err := a.caller.Call(a.facadeName, "", "WatchAPIHostPorts", nil, &result)
	if err != nil {
		return nil, err
	}
	return watcher.NewNotifyWatcher(a.caller, result), nil
}
