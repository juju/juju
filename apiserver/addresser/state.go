// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package addresser

import (
	"github.com/juju/names"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
)

// StateMachine defines the needed methods of state.Machine
// for the work of the Addresser API.

type StateMachine interface {
}

// StetIPAddress defines the needed methods of state.IPAddress
// for the work of the Addresser API.
type StateIPAddress interface {
	state.Entity
	state.EnsureDeader
	state.Remover

	SubnetId() string
	InstanceId() instance.Id
	Address() network.Address
	MACAddress() string
}

// StateInterface defines the needed methods of state.State
// for the work of the Addresser API.
type StateInterface interface {
	state.EnvironAccessor
	state.EntityFinder

	// IPAddress retrieves an IP address by its value.
	IPAddress(value string) (StateIPAddress, error)

	// IPAddress retrieves an IP address by its tag.
	IPAddressByTag(tag names.IPAddressTag) (StateIPAddress, error)

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

func (s stateShim) IPAddressByTag(tag names.IPAddressTag) (StateIPAddress, error) {
	return s.State.IPAddressByTag(tag)
}

var getState = func(st *state.State) StateInterface {
	return stateShim{st}
}
