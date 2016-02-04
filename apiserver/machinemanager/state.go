// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager

import (
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/state"
)

type stateInterface interface {
	ModelConfig() (*config.Config, error)
	Model() (*state.Model, error)
	GetBlockForType(t state.BlockType) (state.Block, bool, error)
	AddOneMachine(template state.MachineTemplate) (*state.Machine, error)
	AddMachineInsideNewMachine(template, parentTemplate state.MachineTemplate, containerType instance.ContainerType) (*state.Machine, error)
	AddMachineInsideMachine(template state.MachineTemplate, parentId string, containerType instance.ContainerType) (*state.Machine, error)
}

type stateShim struct {
	*state.State
}

func (s stateShim) ModelConfig() (*config.Config, error) {
	return s.State.ModelConfig()
}

func (s stateShim) Model() (*state.Model, error) {
	return s.State.Model()
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
