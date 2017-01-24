// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/state"
)

// Backend provides selected methods off the state.State struct
// plus additional helpers.
type Backend interface {
	Application(name string) (*state.Application, error)
	ForModel(modelTag names.ModelTag) (*state.State, error)
	ModelTag() names.ModelTag
	ModelUUID() string
	WatchOfferedApplications() state.StringsWatcher
	ModelName() (string, error)
}

var getStateAccess = func(st *state.State) Backend {
	return &stateShim{st}
}

type stateShim struct {
	*state.State
}

// ModelName returns the name of the model.
func (s *stateShim) ModelName() (string, error) {
	cfg, err := s.ModelConfig()
	if err != nil {
		return "", err
	}
	return cfg.Name(), nil
}
