// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelconfig

import (
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/constraints"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

type Model interface {
	Name() string
	Cloud() (cloud.Cloud, error)
	MachinesLen() (int, error)
}

type model struct {
	*state.Model
}

func (m *model) MachinesLen() (int, error) {
	instanceData, err := m.AllInstanceData()
	if err != nil {
		return 0, err
	}

	return instanceData.Len(), nil
}

// Backend contains the state.State methods used in this package,
// allowing stubs to be created for testing.
type Backend interface {
	common.BlockGetter
	ControllerTag() names.ControllerTag
	Model() Model
	ModelTag() names.ModelTag
	ModelConfigValues() (config.ConfigValues, error)
	UpdateModelConfig(map[string]interface{}, []string, ...state.ValidateConfigFunc) error
	Sequences() (map[string]int, error)
	SetSLA(level, owner string, credentials []byte) error
	SLALevel() (string, error)
	SpaceByName(string) error
	SetModelConstraints(value constraints.Value) error
	ModelConstraints() (constraints.Value, error)
	GetSecretBackend(string) (*coresecrets.SecretBackend, error)
}

type stateShim struct {
	*state.State
	model *state.Model
}

func (st stateShim) UpdateModelConfig(u map[string]interface{}, r []string, a ...state.ValidateConfigFunc) error {
	return st.model.UpdateModelConfig(u, r, a...)
}

func (st stateShim) ModelConfigValues() (config.ConfigValues, error) {
	return st.model.ModelConfigValues()
}

func (st stateShim) ModelTag() names.ModelTag {
	m, err := st.State.Model()
	if err != nil {
		return names.NewModelTag(st.State.ModelUUID())
	}

	return m.ModelTag()
}

func (st stateShim) SpaceByName(name string) error {
	_, err := st.State.SpaceByName(name)
	return err
}

func (st stateShim) GetSecretBackend(name string) (*coresecrets.SecretBackend, error) {
	backends := state.NewSecretBackends(st.State)
	return backends.GetSecretBackend(name)
}

func (st stateShim) Model() Model {
	return &model{st.model}
}

// NewStateBackend creates a backend for the facade to use.
func NewStateBackend(m *state.Model) Backend {
	return stateShim{m.State(), m}
}
