// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"github.com/juju/juju/state"
)

type stateAccess interface {
	Service(name string) (service *state.Service, err error)

	EnvironUUID() string

	WatchOfferedServices() state.StringsWatcher
}

var getStateAccess = func(st *state.State) stateAccess {
	return stateShim{st}
}

type stateShim struct {
	*state.State
}
