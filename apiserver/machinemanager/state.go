// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager

import (
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/state"
)

type stateInterface interface {
	EnvironConfig() (*config.Config, error)
	Environment() (*state.Environment, error)
	GetBlockForType(t state.BlockType) (state.Block, bool, error)
	AddOneMachine(template state.MachineTemplate) (*state.Machine, error)
	AddMachineInsideNewMachine(template, parentTemplate state.MachineTemplate, containerType instance.ContainerType) (*state.Machine, error)
	AddMachineInsideMachine(template state.MachineTemplate, parentId string, containerType instance.ContainerType) (*state.Machine, error)
}

type stateShim struct {
	*state.State
}

func (s stateShim) EnvironConfig() (*config.Config, error) {
	return s.State.EnvironConfig()
}

func (s stateShim) Environment() (*state.Environment, error) {
	return s.State.Environment()
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
