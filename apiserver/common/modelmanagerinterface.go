// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/metricsender"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/description"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	"github.com/juju/juju/status"
)

// ModelManagerBackend defines methods provided by a state
// instance used by the model manager apiserver implementation.
// All the interface methods are defined directly on state.State
// and are reproduced here for use in tests.
type ModelManagerBackend interface {
	APIHostPortsGetter
	ToolsStorageGetter
	BlockGetter
	metricsender.MetricsSenderBackend
	state.CloudAccessor

	ModelUUID() string
	ModelsForUser(names.UserTag) ([]*state.UserModel, error)
	IsControllerAdministrator(user names.UserTag) (bool, error)
	NewModel(state.ModelArgs) (Model, ModelManagerBackend, error)

	ComposeNewModelConfig(modelAttr map[string]interface{}) (map[string]interface{}, error)
	ControllerModel() (Model, error)
	ControllerConfig() (controller.Config, error)
	ForModel(tag names.ModelTag) (ModelManagerBackend, error)
	Model() (Model, error)
	AllModels() ([]Model, error)
	AddModelUser(state.ModelUserSpec) (*state.ModelUser, error)
	RemoveModelUser(names.UserTag) error
	ModelUser(names.UserTag) (*state.ModelUser, error)
	ModelTag() names.ModelTag
	Export() (description.Model, error)
	Close() error
}

// Model defines methods provided by a state.Model instance.
// All the interface methods are defined directly on state.Model
// and are reproduced here for use in tests.
type Model interface {
	Config() (*config.Config, error)
	Life() state.Life
	ModelTag() names.ModelTag
	Owner() names.UserTag
	Status() (status.StatusInfo, error)
	Cloud() string
	CloudCredential() string
	CloudRegion() string
	Users() ([]ModelUser, error)
	Destroy() error
	DestroyIncludingHosted() error
}

type modelManagerStateShim struct {
	*state.State
}

func NewModelManagerBackend(st *state.State) ModelManagerBackend {
	return modelManagerStateShim{st}
}

func (st modelManagerStateShim) ControllerModel() (Model, error) {
	m, err := st.State.ControllerModel()
	if err != nil {
		return nil, err
	}
	return modelShim{m}, nil
}

func (st modelManagerStateShim) NewModel(args state.ModelArgs) (Model, ModelManagerBackend, error) {
	m, otherState, err := st.State.NewModel(args)
	if err != nil {
		return nil, nil, err
	}
	return modelShim{m}, modelManagerStateShim{otherState}, nil
}

func (st modelManagerStateShim) ForModel(tag names.ModelTag) (ModelManagerBackend, error) {
	otherState, err := st.State.ForModel(tag)
	if err != nil {
		return nil, err
	}
	return modelManagerStateShim{otherState}, nil
}

func (st modelManagerStateShim) Model() (Model, error) {
	m, err := st.State.Model()
	if err != nil {
		return nil, err
	}
	return modelShim{m}, nil
}

func (st modelManagerStateShim) AllModels() ([]Model, error) {
	allStateModels, err := st.State.AllModels()
	if err != nil {
		return nil, err
	}
	all := make([]Model, len(allStateModels))
	for i, m := range allStateModels {
		all[i] = modelShim{m}
	}
	return all, nil
}

type modelShim struct {
	*state.Model
}

func (m modelShim) Users() ([]ModelUser, error) {
	stateUsers, err := m.Model.Users()
	if err != nil {
		return nil, err
	}
	users := make([]ModelUser, len(stateUsers))
	for i, user := range stateUsers {
		users[i] = user
	}
	return users, nil
}
