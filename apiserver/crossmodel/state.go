// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/state"
)

// stateAccess provides selected methods off the state.State struct
// plus additional helpers.
type stateAccess interface {
	Application(name string) (service *state.Application, err error)
	ModelTag() names.ModelTag
	ModelUUID() string
	WatchOfferedApplications() state.StringsWatcher
	ModelName() (string, error)
}

var getStateAccess = func(st *state.State) stateAccess {
	return &stateShim{st}
}

type stateShim struct {
	*state.State
}

// EnvironName returns the name of the environment.
func (s *stateShim) ModelName() (string, error) {
	cfg, err := s.ModelConfig()
	if err != nil {
		return "", err
	}
	return cfg.Name(), nil
}
