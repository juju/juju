// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmresources

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	resources "github.com/juju/juju/charmresources"
	"github.com/juju/juju/state"
)

type managerState interface {
	// ResourceManager provides the capability to persist resources.
	ResourceManager() resources.ResourceManager

	// EnvOwner is needed for validation purposes.
	EnvOwner() (names.UserTag, error)

	// GetBlockForType is required to block operations.
	GetBlockForType(t state.BlockType) (state.Block, bool, error)
}

type stateShim struct {
	*state.State
}

var getState = func(st *state.State) managerState {
	return stateShim{st}
}

func (s stateShim) ResourceManager() resources.ResourceManager {
	return s.State.ResourceManager()
}

func (s stateShim) EnvOwner() (names.UserTag, error) {
	env, err := s.State.Environment()
	if err != nil {
		return names.UserTag{}, errors.Trace(err)
	}
	return env.Owner(), nil
}
