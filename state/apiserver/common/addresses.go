// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"launchpad.net/loggo"

	"launchpad.net/juju-core/state/api/params"
)

var logger = loggo.GetLogger("juju.state.apiserver.common")

// AddressAndCertGetter can be used to find out
// state server addresses and the CA public certificate.
type AddressAndCertGetter interface {
	Addresses() ([]string, error)
	APIAddresses() ([]string, error)
	CACert() []byte
}

// APIAddresser implements the APIAddresses method
type APIAddresser struct {
	getter AddressAndCertGetter
}

// NewAPIAddresser returns a new APIAddresser that uses the given getter to
// fetch its addresses.
func NewAPIAddresser(getter AddressAndCertGetter) *APIAddresser {
	return &APIAddresser{getter}
}

// APIAddresses returns the list of addresses used to connect to the API.
func (a *APIAddresser) APIAddresses() (params.StringsResult, error) {
	addrs, err := a.getter.APIAddresses()
	if err != nil {
		return params.StringsResult{}, err
	}
	return params.StringsResult{
		Result: addrs,
	}, nil
}

// Addresser implements a common set of methods for getting state and
// API server addresses, and the CA certificate used to authenticate
// them.
type Addresser struct {
	APIAddresser
}

// NewAddresser returns a new Addresser that uses the given
// st value to fetch its addresses.
func NewAddresser(getter AddressAndCertGetter) *Addresser {
	return &Addresser{APIAddresser{getter}}
}

// StateAddresses returns the list of addresses used to connect to the state.
func (a *Addresser) StateAddresses() (params.StringsResult, error) {
	addrs, err := a.getter.Addresses()
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
		Result: a.getter.CACert(),
	}
}
