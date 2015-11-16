// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

type stateAccess interface {
	// Service gets environment service.
	Service(name string) (service *state.Service, err error)

	// EnvironConfig gets environment configuration.
	EnvironConfig() (*config.Config, error)

	// EnvironUUID gets environment uuid.
	EnvironUUID() string
}

var getStateAccess = func(st *state.State) stateAccess {
	return stateShim{st}
}

type stateShim struct {
	*state.State
}
