// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager

import (
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	"github.com/juju/juju/status"
)

type Backend interface {
	common.APIHostPortsGetter
	common.ModelConfigGetter
	common.ToolsStorageGetter

	ModelUUID() string
	ModelsForUser(names.UserTag) ([]*state.UserModel, error)
	IsControllerAdministrator(user names.UserTag) (bool, error)
	NewModel(state.ModelArgs) (*state.Model, *state.State, error)
	ControllerModel() (*state.Model, error)
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
	Owner() names.UserTag
	Status() (status.StatusInfo, error)
	Users() ([]common.ModelUser, error)
	Cloud() string
	CloudRegion() string
}

type stateShim struct {
	*state.State
}

func NewStateBackend(st *state.State) Backend {
	return stateShim{st}
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
