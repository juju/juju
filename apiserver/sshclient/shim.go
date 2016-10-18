// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package sshclient

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
)

// Backend defines the State API used by the sshclient facade.
type Backend interface {
	ModelConfig() (*config.Config, error)
	GetMachineForEntity(tag string) (SSHMachine, error)
	GetSSHHostKeys(names.MachineTag) (state.SSHHostKeys, error)
	ModelTag() names.ModelTag
}

// SSHMachine specifies the methods on State.Machine of interest to
// the SSHClient facade.
type SSHMachine interface {
	MachineTag() names.MachineTag
	PublicAddress() (network.Address, error)
	PrivateAddress() (network.Address, error)
}

// newFacade wraps New to express the supplied *state.State as a Backend.
func newFacade(st *state.State, res facade.Resources, auth facade.Authorizer) (*Facade, error) {
	return New(&backend{st}, res, auth)
}

type backend struct {
	*state.State
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
		machine, err := b.State.Machine(tag.Id())
		if err != nil {
			return nil, errors.Trace(err)
		}
		return machine, nil
	case names.UnitTag:
		unit, err := b.State.Unit(tag.Id())
		if err != nil {
			return nil, errors.Trace(err)
		}
		machineId, err := unit.AssignedMachineId()
		if err != nil {
			return nil, errors.Trace(err)
		}
		machine, err := b.State.Machine(machineId)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return machine, nil
	default:
		return nil, errors.Errorf("unsupported entity: %q", tagString)
	}
}
