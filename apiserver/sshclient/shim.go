// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package sshclient

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
)

// Backend defines the State API used by the sshclient facade.
type Backend interface {
	ModelConfig() (*config.Config, error)
	GetMachineForTarget(target string) (SSHMachine, error)
	GetSSHHostKeys(names.MachineTag) (state.SSHHostKeys, error)
}

// SSHMachine specifies the methods on State.Machine of interest to
// the SSHClient facade.
type SSHMachine interface {
	MachineTag() names.MachineTag
	PublicAddress() (network.Address, error)
	PrivateAddress() (network.Address, error)
}

// newFacade wraps New to express the supplied *state.State as a Backend.
func newFacade(st *state.State, res *common.Resources, auth common.Authorizer) (*Facade, error) {
	return New(&backend{st}, res, auth)
}

type backend struct {
	*state.State
}

// GetMachineForTarget takes a machine ID or unit name and returns the
// associated SSHMachine.
func (b *backend) GetMachineForTarget(target string) (SSHMachine, error) {
	switch {
	case names.IsValidMachine(target):
		machine, err := b.State.Machine(target)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return machine, nil
	case names.IsValidUnit(target):
		unit, err := b.State.Unit(target)
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
		return nil, errors.Errorf("unsupported target: %q", target)
	}
}
