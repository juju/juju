// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager

import (
	names "gopkg.in/juju/names.v2"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/state"
)

type stateInterface interface {
	ModelConfig() (*config.Config, error)
	Model() (*state.Model, error)
	ModelTag() names.ModelTag
	GetBlockForType(t state.BlockType) (state.Block, bool, error)
	AddOneMachine(template state.MachineTemplate) (*state.Machine, error)
	AddMachineInsideNewMachine(template, parentTemplate state.MachineTemplate, containerType instance.ContainerType) (*state.Machine, error)
	AddMachineInsideMachine(template state.MachineTemplate, parentId string, containerType instance.ContainerType) (*state.Machine, error)

	GetModel(names.ModelTag) (Model, error)
	Cloud(string) (cloud.Cloud, error)
	Clouds() (map[names.CloudTag]cloud.Cloud, error)
	CloudCredentials(user names.UserTag, cloudName string) (map[string]cloud.Credential, error)
	CloudCredential(tag names.CloudCredentialTag) (cloud.Credential, error)
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

func (s stateShim) GetModel(tag names.ModelTag) (Model, error) {
	m, err := s.State.GetModel(tag)
	if err != nil {
		return nil, err
	}
	return m, nil
}

type Model interface {
	Cloud() string
	CloudCredential() (names.CloudCredentialTag, bool)
	CloudRegion() string
	ModelTag() names.ModelTag

	Config() (*config.Config, error)
}
