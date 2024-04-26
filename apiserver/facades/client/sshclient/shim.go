// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshclient

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/core/network"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
)

// Backend defines the State API used by the sshclient facade.
type Backend interface {
	GetMachineForEntity(tag string) (SSHMachine, error)
	GetSSHHostKeys(names.MachineTag) (state.SSHHostKeys, error)
	ModelTag() names.ModelTag
	ControllerTag() names.ControllerTag
	Model() (Model, error)
	CloudSpec(context.Context) (environscloudspec.CloudSpec, error)
}

// Model defines a point of use interface for the model from state.
type Model interface {
	ControllerUUID() string
	Config() (*config.Config, error)
	Type() state.ModelType
}

// Broker is a subset of caas broker.
type Broker interface {
	GetSecretToken(ctx context.Context, name string) (string, error)
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
	stateenvirons.EnvironConfigGetter

	controllerTag  names.ControllerTag
	modelTag       names.ModelTag
	networkService NetworkService
}

// ModelTag returns the model tag of the backend.
func (b *backend) ModelTag() names.ModelTag {
	return b.modelTag
}

func (b *backend) Model() (Model, error) {
	return b.State.Model()
}

func (b *backend) CloudSpec(ctx context.Context) (environscloudspec.CloudSpec, error) {
	return b.EnvironConfigGetter.CloudSpec(ctx)
}

// ControllerTag returns the controller tag of the backend.
func (b *backend) ControllerTag() names.ControllerTag {
	return b.controllerTag
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
