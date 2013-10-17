// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"launchpad.net/juju-core/state/api/params"
)

// AddressAndCertGetter can be used to find out
// state server addresses and the CA public certificate.
// It is implemented by state.State.
type AddressAndCertGetter interface {
	Addresses() ([]string, error)
	APIAddresses() ([]string, error)
	CACert() []byte
}

// Addresser implements a common set of methods for getting state and
// API server addresses, and the CA certificate used to authenticate
// them.
type Addresser struct {
	st AddressAndCertGetter
}

// NewAddresser returns a new Addresser that uses the given
// st value to fetch its addresses.
func NewAddresser(st AddressAndCertGetter) *Addresser {
	return &Addresser{st}
}

// StateAddresses returns the list of addresses used to connect to the state.
//
// TODO(dimitern): Remove this once we have a way to get state/API
// public addresses from state.
// BUG(lp:1205371): This is temporary, until the Addresser worker
// lands and we can take the addresses of all machines with
// JobManageState.
func (a *Addresser) StateAddresses() (params.StringsResult, error) {
	addrs, err := a.st.Addresses()
	if err != nil {
		return params.StringsResult{}, err
	}
	return params.StringsResult{
		Result: addrs,
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
	addrs, err := a.st.APIAddresses()
	if err != nil {
		return params.StringsResult{}, err
	}
	return params.StringsResult{
		Result: addrs,
	}, nil
}

// CACert returns the certificate used to validate the state connection.
func (a *Addresser) CACert() params.BytesResult {
	return params.BytesResult{
		Result: a.st.CACert(),
	}
}
