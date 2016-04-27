// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshclient

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

// NewFacade returns a new Facade based on an existing API connection.
func NewFacade(caller base.APICaller) *Facade {
	return &Facade{base.NewFacadeCaller(caller, "SSHClient")}
}

type Facade struct {
	caller base.FacadeCaller
}

// PublicAddress returns the public address for the SSH target
// provided. The target may be provided as a machine ID or unit name.
func (facade *Facade) PublicAddress(target string) (string, error) {
	addr, err := facade.addressCall("PublicAddress", target)
	return addr, errors.Trace(err)
}

// PrivateAddress returns the private address for the SSH target
// provided. The target may be provided as a machine ID or unit name.
func (facade *Facade) PrivateAddress(target string) (string, error) {
	addr, err := facade.addressCall("PrivateAddress", target)
	return addr, errors.Trace(err)
}

func (facade *Facade) addressCall(callName, target string) (string, error) {
	var out params.SSHAddressResults
	err := facade.caller.FacadeCall(callName, targetToArg(target), &out)
	if err != nil {
		return "", errors.Trace(err)
	}
	if len(out.Results) != 1 {
		return "", countError(len(out.Results))
	}
	if err := out.Results[0].Error; err != nil {
		return "", errors.Trace(err)
	}
	return out.Results[0].Address, nil
}

// PublicKeys returns the SSH public host keys for the SSH target
// provided. The target may be provided as a machine ID or unit name.
func (facade *Facade) PublicKeys(target string) ([]string, error) {
	var out params.SSHPublicKeysResults
	err := facade.caller.FacadeCall("PublicKeys", targetToArg(target), &out)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(out.Results) != 1 {
		return nil, countError(len(out.Results))
	}
	if err := out.Results[0].Error; err != nil {
		return nil, errors.Trace(err)
	}
	return out.Results[0].PublicKeys, nil
}

// Proxy returns whether SSH connections should be proxied through the
// controller hosts for the associated model.
func (facade *Facade) Proxy() (bool, error) {
	var out params.SSHProxyResult
	err := facade.caller.FacadeCall("Proxy", nil, &out)
	if err != nil {
		return false, errors.Trace(err)
	}
	return out.UseProxy, nil
}

func targetToArg(target string) params.SSHTargets {
	return params.SSHTargets{
		Targets: []params.SSHTarget{{target}},
	}
}

// countError complains about malformed results.
func countError(count int) error {
	return errors.Errorf("expected 1 result, got %d", count)
}
