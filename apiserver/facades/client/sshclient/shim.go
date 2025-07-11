// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshclient

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/state"
)

// Backend defines the State API used by the sshclient facade.
type Backend interface {
	GetSSHHostKeys(names.MachineTag) (state.SSHHostKeys, error)
}

// SSHMachine specifies the methods on State.Machine of interest to
// the SSHClient facade.
type SSHMachine interface {
	MachineTag() names.MachineTag
	PublicAddress(context.Context) (network.SpaceAddress, error)
	PrivateAddress(context.Context) (network.SpaceAddress, error)
	AllDeviceSpaceAddresses(context.Context) (network.SpaceAddresses, error)
}

type sshMachine struct {
	machineUUID    machine.UUID
	machineName    machine.Name
	networkService NetworkService
}

func (m *sshMachine) MachineTag() names.MachineTag {
	return names.NewMachineTag(m.machineName.String())
}

func (m *sshMachine) PublicAddress(ctx context.Context) (network.SpaceAddress, error) {
	addrs, err := m.networkService.GetMachineAddresses(ctx, m.machineUUID)
	if err != nil {
		return network.SpaceAddress{}, errors.Trace(err)
	}
	addr, ok := addrs.OneMatchingScope(network.ScopeMatchPublic)
	if !ok {
		return network.SpaceAddress{}, errors.Errorf("no public address found for machine %s", m.machineName)
	}
	return addr, nil
}

func (m *sshMachine) PrivateAddress(ctx context.Context) (network.SpaceAddress, error) {
	addrs, err := m.networkService.GetMachineAddresses(ctx, m.machineUUID)
	if err != nil {
		return network.SpaceAddress{}, errors.Trace(err)
	}
	addr, ok := addrs.OneMatchingScope(network.ScopeMatchCloudLocal)
	if !ok {
		return network.SpaceAddress{}, errors.Errorf("no private address found for machine %s", m.machineName)
	}
	return addr, nil
}

// AllDeviceSpaceAddresses returns all machine link-layer
// device addresses as SpaceAddresses.
func (m *sshMachine) AllDeviceSpaceAddresses(ctx context.Context) (network.SpaceAddresses, error) {
	addrs, err := m.networkService.GetMachineAddresses(ctx, m.machineUUID)
	return addrs, errors.Trace(err)
}
