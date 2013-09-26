// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
)

// EnvironConfigAndCertGetter defines EnvironConfig and CACert
// methods.
type EnvironConfigAndCertGetter interface {
	EnvironConfig() (*config.Config, error)
	CACert() []byte
}

// PasswordChanger implements a common set of methods for getting
// state and API addresses, as well as the CA certificated, used to
// connect to them.
type Addresser struct {
	st EnvironConfigAndCertGetter
}

// NewAddresser returns a new Addresser.
func NewAddresser(st EnvironConfigAndCertGetter) *Addresser {
	return &Addresser{st}
}

// getEnvironStateInfo returns the state and API connection
// information from the state and the environment.
//
// TODO(dimitern): Remove this once we have a way to get state/API
// public addresses from state.
// BUG(lp:1205371): This is temporary, until the Addresser worker
// lands and we can take the addresses of all machines with
// JobManageState.
func (a *Addresser) getEnvironStateInfo() (*state.Info, *api.Info, error) {
	cfg, err := a.st.EnvironConfig()
	if err != nil {
		return nil, nil, err
	}
	env, err := environs.New(cfg)
	if err != nil {
		return nil, nil, err
	}
	return env.StateInfo()
}

// StateAddresses returns the list of addresses used to connect to the state.
//
// TODO(dimitern): Remove this once we have a way to get state/API
// public addresses from state.
// BUG(lp:1205371): This is temporary, until the Addresser worker
// lands and we can take the addresses of all machines with
// JobManageState.
func (a *Addresser) StateAddresses() (params.StringsResult, error) {
	stateInfo, _, err := a.getEnvironStateInfo()
	if err != nil {
		return params.StringsResult{}, err
	}
	return params.StringsResult{
		Result: stateInfo.Addrs,
	}, nil
}

// APIAddresses returns the list of addresses used to connect to the API.
//
// TODO(dimitern): Remove this once we have a way to get state/API
// public addresses from state.
// BUG(lp:1205371): This is temporary, until the Addresser worker
// lands and we can take the addresses of all machines with
// JobManageState.
func (a *Addresser) APIAddresses() (params.StringsResult, error) {
	_, apiInfo, err := a.getEnvironStateInfo()
	if err != nil {
		return params.StringsResult{}, err
	}
	return params.StringsResult{
		Result: apiInfo.Addrs,
	}, nil
}

// CACert returns the certificate used to validate the state connection.
func (a *Addresser) CACert() params.BytesResult {
	return params.BytesResult{
		Result: a.st.CACert(),
	}
}
