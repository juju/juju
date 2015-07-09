// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package addresser

import (
	"github.com/juju/juju/state"
)

type StateIPAddress interface {
	state.Entity
	state.EnsureDeader
	state.Remover
}

type StateInterface interface {
	state.EnvironAccessor
	state.EntityFinder

	IPAddress(value string) (StateIPAddress, error)
	WatchIPAddresses() state.StringsWatcher
}

type stateShim struct {
	*state.State
}

func (s stateShim) IPAddress(value string) (StateIPAddress, error) {
	return s.State.IPAddress(value)
}

var getState = func(st *state.State) StateInterface {
	return stateShim{st}
}
