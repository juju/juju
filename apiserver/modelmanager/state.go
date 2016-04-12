// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager

import (
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	"github.com/juju/juju/status"
)

var getState = func(st *state.State) stateInterface {
	return stateShim{st}
}

type stateInterface interface {
	ModelsForUser(names.UserTag) ([]*state.UserModel, error)
	IsControllerAdministrator(user names.UserTag) (bool, error)
	NewModel(state.ModelArgs) (*state.Model, *state.State, error)
	ControllerModel() (*state.Model, error)
	ForModel(tag names.ModelTag) (*state.State, error)
	GetModel(names.ModelTag) (Model, error)
}

type Model interface {
	Config() (*config.Config, error)
	Life() state.Life
	Owner() names.UserTag
	Status() (status.StatusInfo, error)
	Users() ([]common.ModelUser, error)
}

type stateShim struct {
	*state.State
}

func (st stateShim) GetModel(tag names.ModelTag) (Model, error) {
	m, err := st.State.GetModel(tag)
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
