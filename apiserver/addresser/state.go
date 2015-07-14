// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package addresser

import (
	"github.com/juju/juju/state"
)

// StetIPAddress defines the needed methods of state.IPAddress
// for the work of the Addresser API.
type StateIPAddress interface {
	state.Entity
	state.EnsureDeader
	state.Remover
}

// StateInterface defines the needed methods of state.State
// for the work of the Addresser API.
type StateInterface interface {
	state.EnvironAccessor
	state.EntityFinder

	// IPAddress retrieves an IP address by its value.
	IPAddress(value string) (StateIPAddress, error)

	// WatchIPAddresses notifies about lifecycle changes
	// of IP addresses.
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
