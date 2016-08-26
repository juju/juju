// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package sshclient implements the API endpoint required for Juju
// clients that wish to make SSH connections to Juju managed machines.
package sshclient

import (
	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/description"
	"github.com/juju/juju/network"
)

func init() {
	common.RegisterStandardFacade("SSHClient", 1, newFacade)
}

// Facade implements the API required by the sshclient worker.
type Facade struct {
	backend    Backend
	authorizer facade.Authorizer
}

// New returns a new API facade for the sshclient worker.
func New(backend Backend, _ facade.Resources, authorizer facade.Authorizer) (*Facade, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}
	return &Facade{backend: backend, authorizer: authorizer}, nil
}

func (facade *Facade) checkIsModelAdmin() error {
	isModelAdmin, err := facade.authorizer.HasPermission(description.AdminAccess, facade.backend.ModelTag())
	if err != nil {
		return errors.Trace(err)
	}
	if !isModelAdmin {
		return common.ErrPerm
	}
	return nil
}

// PublicAddress reports the preferred public network address for one
// or more entities. Machines and units are suppored.
func (facade *Facade) PublicAddress(args params.Entities) (params.SSHAddressResults, error) {
	if err := facade.checkIsModelAdmin(); err != nil {
		return params.SSHAddressResults{}, errors.Trace(err)
	}

	getter := func(m SSHMachine) (network.Address, error) { return m.PublicAddress() }
	return facade.getAddresses(args, getter)
}

// PrivateAddress reports the preferred private network address for one or
// more entities. Machines and units are supported.
func (facade *Facade) PrivateAddress(args params.Entities) (params.SSHAddressResults, error) {
	if err := facade.checkIsModelAdmin(); err != nil {
		return params.SSHAddressResults{}, errors.Trace(err)
	}

	getter := func(m SSHMachine) (network.Address, error) { return m.PrivateAddress() }
	return facade.getAddresses(args, getter)
}

func (facade *Facade) getAddresses(args params.Entities, getter func(SSHMachine) (network.Address, error)) (
	params.SSHAddressResults, error,
) {
	out := params.SSHAddressResults{
		Results: make([]params.SSHAddressResult, len(args.Entities)),
	}
	for i, entity := range args.Entities {
		machine, err := facade.backend.GetMachineForEntity(entity.Tag)
		if err != nil {
			out.Results[i].Error = common.ServerError(err)
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

// PublicKeys returns the public SSH hosts for one or more
// entities. Machines and units are supported.
func (facade *Facade) PublicKeys(args params.Entities) (params.SSHPublicKeysResults, error) {
	if err := facade.checkIsModelAdmin(); err != nil {
		return params.SSHPublicKeysResults{}, errors.Trace(err)
	}

	out := params.SSHPublicKeysResults{
		Results: make([]params.SSHPublicKeysResult, len(args.Entities)),
	}
	for i, entity := range args.Entities {
		machine, err := facade.backend.GetMachineForEntity(entity.Tag)
		if err != nil {
			out.Results[i].Error = common.ServerError(err)
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
	if err := facade.checkIsModelAdmin(); err != nil {
		return params.SSHProxyResult{}, errors.Trace(err)
	}
	config, err := facade.backend.ModelConfig()
	if err != nil {
		return params.SSHProxyResult{}, err
	}
	return params.SSHProxyResult{UseProxy: config.ProxySSH()}, nil
}
