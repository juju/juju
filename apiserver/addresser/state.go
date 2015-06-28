// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package addresser

import (
	"github.com/juju/names"
	"github.com/juju/utils"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
)

// StateIPAddress defines the methods needed from an IP address in state.
type StateIPAddress interface {
	Value() string
	UUID() (utils.UUID, error)
	Tag() names.Tag
	SubnetId() string
	InstanceId() instance.Id
	Type() network.AddressType
	Scope() network.Scope
	Life() state.Life

	Remove() error
}

// StateInterface defines the needed access methods to state.
type StateInterface interface {
	EnvironConfig() (*config.Config, error)
	FindEntity(tag names.Tag) (state.Entity, error)
	WatchIPAddresses() state.StringsWatcher
}

// stateShim encapsulates the state access.
type stateShim struct {
	*state.State
}

// FindTag finds an entity by tag.
func (s stateShim) FindEntity(tag names.Tag) (state.Entity, error) {
	return s.State.FindEntity(tag)
}

// getState is a mockable access to state.
var getState = func(st *state.State) StateInterface {
	return stateShim{st}
}
