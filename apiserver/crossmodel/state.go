// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"github.com/juju/names"

	"github.com/juju/juju/state"
)

// stateAccess provides selected methods off the state.State struct
// plus additional helpers.
type stateAccess interface {
	Service(name string) (service *state.Service, err error)
	EnvironTag() names.EnvironTag
	EnvironUUID() string
	WatchOfferedServices() state.StringsWatcher
	EnvironName() (string, error)
}

var getStateAccess = func(st *state.State) stateAccess {
	return &stateShim{st}
}

type stateShim struct {
	*state.State
}

// EnvironName returns the name of the environment.
func (s *stateShim) EnvironName() (string, error) {
	cfg, err := s.EnvironConfig()
	if err != nil {
		return "", err
	}
	return cfg.Name(), nil
}
