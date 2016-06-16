// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager

import (
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	"github.com/juju/juju/status"
)

type Backend interface {
	environs.EnvironConfigGetter
	common.APIHostPortsGetter
	common.ToolsStorageGetter

	Cloud() (cloud.Cloud, error)
	CloudCredentials(names.UserTag) (map[string]cloud.Credential, error)
	ModelUUID() string
	ModelsForUser(names.UserTag) ([]*state.UserModel, error)
	IsControllerAdministrator(user names.UserTag) (bool, error)
	NewModel(state.ModelArgs) (Model, Backend, error)
	ControllerModel() (Model, error)
	ControllerConfig() (controller.Config, error)
	ForModel(tag names.ModelTag) (Backend, error)
	Model() (Model, error)
	AddModelUser(state.ModelUserSpec) (*state.ModelUser, error)
	RemoveModelUser(names.UserTag) error
	ModelUser(names.UserTag) (*state.ModelUser, error)
	Close() error
}

type Model interface {
	Config() (*config.Config, error)
	Life() state.Life
	ModelTag() names.ModelTag
	Owner() names.UserTag
	Status() (status.StatusInfo, error)
	CloudCredential() string
	CloudRegion() string
	Users() ([]common.ModelUser, error)
}

type stateShim struct {
	*state.State
}

func NewStateBackend(st *state.State) Backend {
	return stateShim{st}
}

func (st stateShim) ControllerModel() (Model, error) {
	m, err := st.State.ControllerModel()
	if err != nil {
		return nil, err
	}
	return modelShim{m}, nil
}

func (st stateShim) NewModel(args state.ModelArgs) (Model, Backend, error) {
	m, otherState, err := st.State.NewModel(args)
	if err != nil {
		return nil, nil, err
	}
	return modelShim{m}, stateShim{otherState}, nil
}

func (st stateShim) ForModel(tag names.ModelTag) (Backend, error) {
	otherState, err := st.State.ForModel(tag)
	if err != nil {
		return nil, err
	}
	return stateShim{otherState}, nil
}

func (st stateShim) Model() (Model, error) {
	m, err := st.State.Model()
	if err != nil {
		return nil, err
	}
	return modelShim{m}, nil
}

type modelShim struct {
	*state.Model
}

func (m modelShim) Users() ([]common.ModelUser, error) {
	stateUsers, err := m.Model.Users()
	if err != nil {
		return nil, err
	}
	users := make([]common.ModelUser, len(stateUsers))
	for i, user := range stateUsers {
		users[i] = user
	}
	return users, nil
}
