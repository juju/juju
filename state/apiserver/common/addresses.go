// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"time"

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

// Addresser implements a common set of methods for getting state and
// API server addresses, and the CA certificate used to authenticate
// them.
type Addresser struct {
	st EnvironConfigAndCertGetter
	cache map[string]interface{}
}

const addressTimeout = 1*time.Minute

type cachedAddress struct {
	expiry	time.Time
	stateInfo state.Info
	apiInfo	api.Info

}

var AddressCache = make(map[string]interface{})

// NewAddresser returns a new Addresser.
func NewAddresser(st EnvironConfigAndCertGetter) *Addresser {
	return &Addresser{st, AddressCache}
}

func NewAPIAddresser(st EnvironConfigAndCertGetter) *APIAddresser {
	return &APIAddresser{st, AddressCache}
}

// getEnvironStateInfo returns the state and API connection
// information from the state and the environment.
//
// TODO(dimitern): Remove this once we have a way to get state/API
// public addresses from state.
// BUG(lp:1205371): This is temporary, until the Addresser worker
// lands and we can take the addresses of all machines with
// JobManageState.
func getEnvironStateInfo(st EnvironConfigAndCertGetter, cache map[string]interface{}) (*state.Info, *api.Info, error) {
	if val, ok := cache["environ-state-info"]; ok {
		if cached, ok := val.(cachedAddress); ok {
			if time.Now().Before(cached.expiry) {
				return &cached.stateInfo, &cached.apiInfo, nil
			}
		}
		delete(cache, "environ-state-info")
	}
	cfg, err := st.EnvironConfig()
	if err != nil {
		return nil, nil, err
	}
	env, err := environs.New(cfg)
	if err != nil {
		return nil, nil, err
	}
	stateInfo, apiInfo, err := env.StateInfo()
	if err != nil {
		return nil, nil, err
	}
	cache["environ-state-info"] = cachedAddress{
		expiry: time.Now().Add(addressTimeout),
		stateInfo: *stateInfo,
		apiInfo: *apiInfo,
	}
	return stateInfo, apiInfo, nil
}

func (a *Addresser) getEnvironStateInfo() (*state.Info, *api.Info, error) {
	return getEnvironStateInfo(a.st, a.cache)
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

type APIAddresser struct {
	st EnvironConfigAndCertGetter
	cache map[string]interface{}
}

func (a *APIAddresser) getEnvironStateInfo() (*state.Info, *api.Info, error) {
	return getEnvironStateInfo(a.st, a.cache)
}

func (a *APIAddresser) APIAddresses() (params.StringsResult, error) {
	_, apiInfo, err := a.getEnvironStateInfo()
	if err != nil {
		return params.StringsResult{}, err
	}
	return params.StringsResult{
		Result: apiInfo.Addrs,
	}, nil
}

