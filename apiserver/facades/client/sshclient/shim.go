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
	GetMachineForEntity(tag string) (SSHMachine, error)
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

// NetworkService is the interface that is used to interact with the
// network spaces/subnets.
type NetworkService interface {
	// GetAllSubnets returns all the subnets for the model.
	GetAllSubnets(ctx context.Context) (network.SubnetInfos, error)
}

type sshMachine struct {
	*state.Machine

	st             *state.State
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

type backend struct {
	*state.State

	networkService NetworkService
}

// GetMachineForEntity takes a machine or unit tag (as a string) and
// returns the associated SSHMachine.
func (b *backend) GetMachineForEntity(tagString string) (SSHMachine, error) {
	tag, err := names.ParseTag(tagString)
	if err != nil {
		return nil, errors.Trace(err)
	}

	switch tag := tag.(type) {
	case names.MachineTag:
		m, err := b.State.Machine(tag.Id())
		if err != nil {
			return nil, errors.Trace(err)
		}
		return &sshMachine{Machine: m, st: b.State, networkService: b.networkService}, nil
	case names.UnitTag:
		unit, err := b.State.Unit(tag.Id())
		if err != nil {
			return nil, errors.Trace(err)
		}
		machineId, err := unit.AssignedMachineId()
		if err != nil {
			return nil, errors.Trace(err)
		}
		m, err := b.State.Machine(machineId)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return &sshMachine{Machine: m, st: b.State, networkService: b.networkService}, nil
	default:
		return nil, errors.Errorf("unsupported entity: %q", tagString)
	}
}
