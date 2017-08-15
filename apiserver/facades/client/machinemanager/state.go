// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager

import (
	names "gopkg.in/juju/names.v2"

	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/common/storagecommon"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/state"
)

type Backend interface {
	storagecommon.StorageInterface
	state.CloudAccessor

	Machine(string) (Machine, error)
	ModelConfig() (*config.Config, error)
	Model() (Model, error)
	ModelTag() names.ModelTag
	GetBlockForType(t state.BlockType) (state.Block, bool, error)
	AddOneMachine(template state.MachineTemplate) (*state.Machine, error)
	AddMachineInsideNewMachine(template, parentTemplate state.MachineTemplate, containerType instance.ContainerType) (*state.Machine, error)
	AddMachineInsideMachine(template state.MachineTemplate, parentId string, containerType instance.ContainerType) (*state.Machine, error)
}

type Pool interface {
	GetModel(string) (Model, func(), error)
}

type Model interface {
	Name() string
	UUID() string
	Cloud() string
	CloudCredential() (names.CloudCredentialTag, bool)
	CloudRegion() string
	Config() (*config.Config, error)
}

type Machine interface {
	Destroy() error
	ForceDestroy() error
	Units() ([]Unit, error)
}

type stateShim struct {
	*state.State
	*state.IAASModel
}

func (s stateShim) Machine(name string) (Machine, error) {
	m, err := s.State.Machine(name)
	if err != nil {
		return nil, err
	}
	return machineShim{m}, nil
}

func (s stateShim) Model() (Model, error) {
	return s.State.Model()
}

type poolShim struct {
	pool *state.StatePool
}

func (p *poolShim) GetModel(uuid string) (Model, func(), error) {
	m, release, err := p.pool.GetModel(uuid)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	return m, func() { release() }, nil
}

type machineShim struct {
	*state.Machine
}

func (m machineShim) Units() ([]Unit, error) {
	units, err := m.Machine.Units()
	if err != nil {
		return nil, err
	}
	out := make([]Unit, len(units))
	for i, u := range units {
		out[i] = u
	}
	return out, nil
}

type Unit interface {
	UnitTag() names.UnitTag
}
