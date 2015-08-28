// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environmentmanager

import (
	"github.com/juju/names"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

var getState = func(st *state.State) stateInterface {
	return stateShim{st}
}

type stateInterface interface {
	EnvironmentsForUser(names.UserTag) ([]*state.UserEnvironment, error)
	IsSystemAdministrator(user names.UserTag) (bool, error)
	NewEnvironment(*config.Config, names.UserTag) (*state.Environment, *state.State, error)
	StateServerEnvironment() (*state.Environment, error)
}

type stateShim struct {
	*state.State
}
