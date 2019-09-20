// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller

import (
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
)

// StateMachine represents a machine from state package.
type StateMachine interface {
	state.Entity

	Id() string
	InstanceId() (instance.Id, error)
	ProviderAddresses() network.SpaceAddresses
	SetProviderAddresses(...network.SpaceAddress) error
	InstanceStatus() (status.StatusInfo, error)
	SetInstanceStatus(status.StatusInfo) error
	SetStatus(status.StatusInfo) error
	String() string
	Refresh() error
	Life() state.Life
	Status() (status.StatusInfo, error)
	IsManual() (bool, error)
}

type StateInterface interface {
	state.ModelAccessor
	state.ModelMachinesWatcher
	state.EntityFinder
	network.SpaceLookup

	Machine(id string) (StateMachine, error)
}

// TODO - CAAS(ericclaudejones): This should contain state alone, model will be
// removed once all relevant methods are moved from state to model.
type stateShim struct {
	*state.State
	*state.Model
}

func (s stateShim) Machine(id string) (StateMachine, error) {
	return s.State.Machine(id)
}

var getState = func(st *state.State, m *state.Model) StateInterface {
	return stateShim{st, m}
}
