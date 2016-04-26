// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package sshclient implements the API endpoint required for Juju
// clients that wish to make SSH connections to Juju managed machines.
package sshclient

import (
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
)

func init() {
	common.RegisterStandardFacade("SSHClient", 1, newFacade)
}

// Facade implements the API required by the sshclient worker.
type Facade struct {
	backend Backend
}

// New returns a new API facade for the sshclient worker.
func New(backend Backend, _ *common.Resources, authorizer common.Authorizer) (*Facade, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}
	return &Facade{backend: backend}, nil
}

// PublicAddress reports the best public network address for one or
// more SSH targets. Targets may be provided as either a machine ID or
// unit name.
func (facade *Facade) PublicAddress(args params.SSHTargets) (params.SSHAddressResults, error) {
	getter := func(m SSHMachine) (network.Address, error) { return m.PublicAddress() }
	return facade.getAddresses(args, getter)
}

// PrivateAddress reports the best private network address for one or
// more SSH targets. Targets may be provided as either a machine ID or
// unit name.
func (facade *Facade) PrivateAddress(args params.SSHTargets) (params.SSHAddressResults, error) {
	getter := func(m SSHMachine) (network.Address, error) { return m.PrivateAddress() }
	return facade.getAddresses(args, getter)
}

func (facade *Facade) getAddresses(args params.SSHTargets, getter func(SSHMachine) (network.Address, error)) (
	params.SSHAddressResults, error,
) {
	out := params.SSHAddressResults{
		Results: make([]params.SSHAddressResult, len(args.Targets)),
	}
	for i, target := range args.Targets {
		machine, err := facade.backend.GetMachineForTarget(target.Target)
		if err != nil {
			out.Results[i].Error = common.ServerError(common.ErrPerm)
		} else {
			address, err := getter(machine)
			if err != nil {
				out.Results[i].Error = common.ServerError(err)
			} else {
				out.Results[i].Address = address.Value
			}
		}
	}
	return out, nil
}

// PublicKeys returns the public SSH hosts for one or more SSH
// targets. Targets may be provided as either a machine ID or unit
// name.
func (facade *Facade) PublicKeys(args params.SSHTargets) (params.SSHPublicKeysResults, error) {
	out := params.SSHPublicKeysResults{
		Results: make([]params.SSHPublicKeysResult, len(args.Targets)),
	}
	for i, target := range args.Targets {
		machine, err := facade.backend.GetMachineForTarget(target.Target)
		if err != nil {
			out.Results[i].Error = common.ServerError(common.ErrPerm)
		} else {
			keys, err := facade.backend.GetSSHHostKeys(machine.MachineTag())
			if err != nil {
				out.Results[i].Error = common.ServerError(err)
			} else {
				out.Results[i].PublicKeys = []string(keys)
			}
		}
	}
	return out, nil
}

// Proxy returns whether SSH connections should be proxied through the
// controller hosts for the model associated with the API connection.
func (facade *Facade) Proxy() (params.SSHProxyResult, error) {
	config, err := facade.backend.ModelConfig()
	if err != nil {
		return params.SSHProxyResult{}, err
	}
	return params.SSHProxyResult{UseProxy: config.ProxySSH()}, nil
}
