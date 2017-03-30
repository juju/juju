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
	"github.com/juju/juju/network"
	"github.com/juju/juju/permission"
)

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
	isModelAdmin, err := facade.authorizer.HasPermission(permission.AdminAccess, facade.backend.ModelTag())
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
	return facade.getAddressPerEntity(args, getter)
}

// PrivateAddress reports the preferred private network address for one or
// more entities. Machines and units are supported.
func (facade *Facade) PrivateAddress(args params.Entities) (params.SSHAddressResults, error) {
	if err := facade.checkIsModelAdmin(); err != nil {
		return params.SSHAddressResults{}, errors.Trace(err)
	}

	getter := func(m SSHMachine) (network.Address, error) { return m.PrivateAddress() }
	return facade.getAddressPerEntity(args, getter)
}

// AllAddresses reports all addresses known to Juju for each given entity in
// args. Machines and units are supported as entity types. Since the returned
// addresses are gathered from multiple sources, results may include duplicates.
func (facade *Facade) AllAddresses(args params.Entities) (params.SSHAddressesResults, error) {
	if err := facade.checkIsModelAdmin(); err != nil {
		return params.SSHAddressesResults{}, errors.Trace(err)
	}

	getter := func(m SSHMachine) ([]network.Address, error) {
		devicesAddresses, err := m.AllNetworkAddresses()
		if err != nil {
			return nil, errors.Trace(err)
		}

		legacyAddresses := m.Addresses()
		return append(devicesAddresses, legacyAddresses...), nil
	}

	return facade.getAllEntityAddresses(args, getter)
}

func (facade *Facade) getAllEntityAddresses(args params.Entities, getter func(SSHMachine) ([]network.Address, error)) (
	params.SSHAddressesResults, error,
) {
	out := params.SSHAddressesResults{
		Results: make([]params.SSHAddressesResult, len(args.Entities)),
	}
	for i, entity := range args.Entities {
		machine, err := facade.backend.GetMachineForEntity(entity.Tag)
		if err != nil {
			out.Results[i].Error = common.ServerError(err)
		} else {
			addresses, err := getter(machine)
			if err != nil {
				out.Results[i].Error = common.ServerError(err)
				continue
			}

			out.Results[i].Addresses = make([]string, len(addresses))
			for j := range addresses {
				out.Results[i].Addresses[j] = addresses[j].Value
			}
		}
	}
	return out, nil
}

func (facade *Facade) getAddressPerEntity(args params.Entities, addressGetter func(SSHMachine) (network.Address, error)) (
	params.SSHAddressResults, error,
) {
	out := params.SSHAddressResults{
		Results: make([]params.SSHAddressResult, len(args.Entities)),
	}

	getter := func(m SSHMachine) ([]network.Address, error) {
		address, err := addressGetter(m)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return []network.Address{address}, nil
	}

	fullResults, err := facade.getAllEntityAddresses(args, getter)
	if err != nil {
		return params.SSHAddressResults{}, errors.Trace(err)
	}

	for i, result := range fullResults.Results {
		if result.Error != nil {
			out.Results[i].Error = result.Error
		} else {
			out.Results[i].Address = result.Addresses[0]
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
