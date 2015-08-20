// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package addresser

import (
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
)

// StateIPAddress defines the needed methods of state.IPAddress
// for the work of the Addresser API.
type StateIPAddress interface {
	state.Entity
	state.EnsureDeader
	state.Remover

	Value() string
	Life() state.Life
	SubnetId() string
	InstanceId() instance.Id
	Address() network.Address
	MACAddress() string
}

// StateInterface defines the needed methods of state.State
// for the work of the Addresser API.
type StateInterface interface {
	// EnvironConfig retrieves the environment configuration.
	EnvironConfig() (*config.Config, error)

	// DeadIPAddresses retrieves all dead IP addresses.
	DeadIPAddresses() ([]StateIPAddress, error)

	// IPAddress retrieves an IP address by its value.
	IPAddress(value string) (StateIPAddress, error)

	// WatchIPAddresses notifies about lifecycle changes
	// of IP addresses.
	WatchIPAddresses() state.StringsWatcher
}

type stateShim struct {
	*state.State
}

func (s stateShim) DeadIPAddresses() ([]StateIPAddress, error) {
	ipAddresses, err := s.State.DeadIPAddresses()
	if err != nil {
		return nil, err
	}
	// Convert []*state.IPAddress into []StateIPAddress. Direct
	// casts of complete slices are not possible.
	stateIPAddresses := make([]StateIPAddress, len(ipAddresses))
	for i, ipAddress := range ipAddresses {
		stateIPAddresses[i] = StateIPAddress(ipAddress)
	}
	return stateIPAddresses, nil
}

func (s stateShim) IPAddress(value string) (StateIPAddress, error) {
	return s.State.IPAddress(value)
}

var getState = func(st *state.State) StateInterface {
	return stateShim{st}
}
