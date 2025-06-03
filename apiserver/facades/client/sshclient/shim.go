// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshclient

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/state"
)

// Backend defines the State API used by the sshclient facade.
type Backend interface {
	Machine(id string) (*state.Machine, error)
	GetSSHHostKeys(names.MachineTag) (state.SSHHostKeys, error)
}

// SSHMachine specifies the methods on State.Machine of interest to
// the SSHClient facade.
type SSHMachine interface {
	MachineTag() names.MachineTag
	PublicAddress() (network.SpaceAddress, error)
	PrivateAddress() (network.SpaceAddress, error)
	Addresses() network.SpaceAddresses
	AllDeviceSpaceAddresses(context.Context) (network.SpaceAddresses, error)
}

type sshMachine struct {
	*state.Machine

	networkService NetworkService
}

// AllDeviceSpaceAddresses returns all machine link-layer
// device addresses as SpaceAddresses.
func (m *sshMachine) AllDeviceSpaceAddresses(ctx context.Context) (network.SpaceAddresses, error) {
	addrs, err := m.Machine.AllDeviceAddresses()
	if err != nil {
		return nil, errors.Trace(err)
	}

	subs, err := m.networkService.GetAllSubnets(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	spaceAddrs := make(network.SpaceAddresses, len(addrs))
	for i, addr := range addrs {
		if spaceAddrs[i], err = network.ConvertToSpaceAddress(addr, subs); err != nil {
			return nil, errors.Trace(err)
		}
	}
	return spaceAddrs, nil
}
