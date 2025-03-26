// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshclient

import (
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"golang.org/x/crypto/ssh"

	"github.com/juju/juju/core/network"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
)

// Backend defines the State API used by the sshclient facade.
type Backend interface {
	ModelConfig() (*config.Config, error)
	GetMachineForEntity(tag string) (SSHMachine, error)
	GetSSHHostKeys(names.MachineTag) (state.SSHHostKeys, error)
	ModelTag() names.ModelTag
	ControllerTag() names.ControllerTag
	Model() (Model, error)
	CloudSpec() (environscloudspec.CloudSpec, error)

	// SSHServerHostKey returns the public host key for the SSH server.
	// This key was set during the controller bootstrap process via
	// bootstrap-state and is currently a FIXED value.
	SSHServerHostKey() (string, error)

	// UnitVirtualHostKeySSHAuthKeyFormat calls the underlying UnitVirtualHostKey state method
	// and encodes the result into SSH authorised key format.
	UnitVirtualHostKeySSHAuthKeyFormat(unitID string) (string, error)

	// MachineVirtualHostKeySSHAuthKeyFormat calls the underlying MachineVirtualHostKey state method
	// and encodes the result into SSH authorised key format.
	MachineVirtualHostKeySSHAuthKeyFormat(machineID string) (string, error)
}

// Model defines a point of use interface for the model from state.
type Model interface {
	ControllerUUID() string
	Config() (*config.Config, error)
	Type() state.ModelType
}

// Broker is a subset of caas broker.
type Broker interface {
	GetSecretToken(name string) (string, error)
}

// SSHMachine specifies the methods on State.Machine of interest to
// the SSHClient facade.
type SSHMachine interface {
	MachineTag() names.MachineTag
	PublicAddress() (network.SpaceAddress, error)
	PrivateAddress() (network.SpaceAddress, error)
	Addresses() network.SpaceAddresses
	AllDeviceSpaceAddresses() (network.SpaceAddresses, error)
}

type sshMachine struct {
	*state.Machine

	st *state.State
}

// AllDeviceSpaceAddresses returns all machine link-layer
// device addresses as SpaceAddresses.
func (m *sshMachine) AllDeviceSpaceAddresses() (network.SpaceAddresses, error) {
	addrs, err := m.Machine.AllDeviceAddresses()
	if err != nil {
		return nil, errors.Trace(err)
	}

	subs, err := m.st.AllSubnetInfos()
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

	controllerTag names.ControllerTag
	modelTag      names.ModelTag
}

// ModelTag returns the model tag of the backend.
func (b *backend) ModelTag() names.ModelTag {
	return b.modelTag
}

func (b *backend) Model() (Model, error) {
	return b.State.Model()
}

func (b *backend) CloudSpec() (environscloudspec.CloudSpec, error) {
	return b.EnvironConfigGetter.CloudSpec()
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
		return &sshMachine{Machine: m, st: b.State}, nil
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
		return &sshMachine{Machine: m, st: b.State}, nil
	default:
		return nil, errors.Errorf("unsupported entity: %q", tagString)
	}
}

// UnitVirtualHostKeySSHAuthKeyFormat calls the underlying UnitVirtualHostKey state method
// and encodes the result into SSH authorised key format.
func (b *backend) UnitVirtualHostKeySSHAuthKeyFormat(unitID string) (string, error) {
	// The keys are persisted PEM encoded.
	vhk, err := b.State.UnitVirtualHostKey(unitID)
	if err != nil {
		return "", errors.Trace(err)
	}
	signer, err := ssh.ParsePrivateKey(vhk.HostKey())
	if err != nil {
		return "", errors.Trace(err)
	}

	return string(ssh.MarshalAuthorizedKey(signer.PublicKey())), nil
}

// MachineVirtualHostKeySSHAuthKeyFormat calls the underlying MachineVirtualHostKey state method
// and encodes the result into SSH authorised key format.
func (b *backend) MachineVirtualHostKeySSHAuthKeyFormat(machineID string) (string, error) {
	vhk, err := b.State.MachineVirtualHostKey(machineID)
	if err != nil {
		return "", errors.Trace(err)
	}

	signer, err := ssh.ParsePrivateKey(vhk.HostKey())
	if err != nil {
		return "", errors.Trace(err)
	}

	return string(ssh.MarshalAuthorizedKey(signer.PublicKey())), nil
}
