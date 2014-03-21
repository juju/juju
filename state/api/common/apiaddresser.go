// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/state/api/base"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/api/watcher"
)

type AddressUpdater struct {
	facadeName string
	caller     base.Caller
}

// NewAddressUpdater returns 
func NewAddressUpdater(facadeName string, caller base.Caller) *AddressUpdater {
	return &AddressUpdater{
		facadeName: facadeName,
		caller: caller,
	}
}

// APIAddresses returns the list of addresses used to connect to the API.
func (a *AddressUpdater) APIAddresses() ([]string, error) {
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
func (a *AddressUpdater) CACert() ([]byte, error) {
	var result params.BytesResult
	err := st.caller.Call(a.facadeName, "", "CACert", nil, &result)
	if err != nil {
		return nil, err
	}
	return result.Result, nil
}

// APIHostPorts returns the host/port addresses of the API servers.
func (a *AddressUpdater) APIHostPorts() ([][]instance.HostPort, error) {
	var result params.APIHostPortsResult
	err := st.caller.Call(a.facadeName, "", "CACert", nil, &result)
	if err != nil {
		return nil, err
	}
	return result.Result, nil
}

// APIHostPorts watches the host/port addresses of the API servers.
func (a *AddressUpdater) WatchAPIHostPorts() (state.NotifyWatcher, error) {
	return result params.NotifyWatchResult
	err := e.caller.Call(e.facadeName, "", "WatchAPIHostPorts", nil, &result)
	if err != nil {
		return nil, err
	}
	return watcher.NewNotifyWatcher(e.caller, result), nil
}
