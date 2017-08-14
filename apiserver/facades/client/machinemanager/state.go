// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager

import (
	names "gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common/storagecommon"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/state"
)

type stateInterface interface {
	storagecommon.StorageInterface

	Machine(string) (Machine, error)
	ModelConfig() (*config.Config, error)
	Model() (Model, error)
	ModelTag() names.ModelTag
	GetBlockForType(t state.BlockType) (state.Block, bool, error)
	AddOneMachine(template state.MachineTemplate) (*state.Machine, error)
	AddMachineInsideNewMachine(template, parentTemplate state.MachineTemplate, containerType instance.ContainerType) (*state.Machine, error)
	AddMachineInsideMachine(template state.MachineTemplate, parentId string, containerType instance.ContainerType) (*state.Machine, error)

	Cloud(string) (cloud.Cloud, error)
	Clouds() (map[names.CloudTag]cloud.Cloud, error)
	CloudCredentials(user names.UserTag, cloudName string) (map[string]cloud.Credential, error)
	CloudCredential(tag names.CloudCredentialTag) (cloud.Credential, error)
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

func (s stateShim) ModelConfig() (*config.Config, error) {
	return s.State.ModelConfig()
}

func (s stateShim) Model() (Model, error) {
	return s.State.Model()
}
func (s stateShim) ModelTag() names.ModelTag {
	return s.State.ModelTag()
}

func (s stateShim) GetBlockForType(t state.BlockType) (state.Block, bool, error) {
	return s.State.GetBlockForType(t)
}

func (s stateShim) AddOneMachine(template state.MachineTemplate) (*state.Machine, error) {
	return s.State.AddOneMachine(template)
}

func (s stateShim) AddMachineInsideNewMachine(template, parentTemplate state.MachineTemplate, containerType instance.ContainerType) (*state.Machine, error) {
	return s.State.AddMachineInsideNewMachine(template, parentTemplate, containerType)
}

func (s stateShim) AddMachineInsideMachine(template state.MachineTemplate, parentId string, containerType instance.ContainerType) (*state.Machine, error) {
	return s.State.AddMachineInsideMachine(template, parentId, containerType)
}

type Model interface {
	Name() string
	UUID() string
	Cloud() string
	CloudCredential() (names.CloudCredentialTag, bool)
	CloudRegion() string
	ModelTag() names.ModelTag
	Config() (*config.Config, error)
}

type Machine interface {
	Destroy() error
	ForceDestroy() error
	Units() ([]Unit, error)
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
		out[i] = unitShim{u}
	}
	return out, nil
}

type Unit interface {
	UnitTag() names.UnitTag
}

type unitShim struct {
	*state.Unit
}
