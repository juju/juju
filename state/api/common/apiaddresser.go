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
func (st *State) CACert() ([]byte, error) {
	var result params.BytesResult
	err := st.caller.Call("Deployer", "", "CACert", nil, &result)
	if err != nil {
		return nil, err
	}
	return result.Result, nil
}
