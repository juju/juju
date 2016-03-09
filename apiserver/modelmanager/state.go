// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager

import (
	"github.com/juju/names"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

var getState = func(st *state.State) stateInterface {
	return stateShim{st}
}

type stateInterface interface {
	ModelsForUser(names.UserTag) ([]*state.UserModel, error)
	IsControllerAdministrator(user names.UserTag) (bool, error)
	NewModel(*config.Config, names.UserTag) (*state.Model, *state.State, error)
	ControllerModel() (*state.Model, error)
}

type stateShim struct {
	*state.State
}
