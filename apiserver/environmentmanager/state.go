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
	AllMachines() ([]*state.Machine, error)
	Close() error
	Environment() (*state.Environment, error)
	EnvironmentsForUser(names.UserTag) ([]*state.UserEnvironment, error)
	EnvironmentUser(names.UserTag) (*state.EnvironmentUser, error)
	EnvironConfig() (*config.Config, error)
	EnvironUUID() string
	ForEnviron(names.EnvironTag) (*state.State, error)
	GetBlockForType(state.BlockType) (state.Block, bool, error)
	NewEnvironment(*config.Config, names.UserTag) (*state.Environment, *state.State, error)
	RemoveAllEnvironDocs() error
	StateServerEnvironment() (*state.Environment, error)
}

type stateShim struct {
	*state.State
}
