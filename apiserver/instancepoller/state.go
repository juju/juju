// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller

import (
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
)

type StateMachine interface {
	state.Entity

	Id() string
	InstanceId() (instance.Id, error)
	ProviderAddresses() []network.Address
	SetProviderAddresses(...network.Address) error
	InstanceStatus() (string, error)
	SetInstanceStatus(status string) error
	String() string
	Refresh() error
	Life() state.Life
	Status() (state.StatusInfo, error)
	IsManual() (bool, error)
}

type StateInterface interface {
	state.EnvironAccessor
	state.EnvironMachinesWatcher
	state.EntityFinder

	Machine(id string) (StateMachine, error)
}

type stateShim struct {
	*state.State
}

func (s stateShim) Machine(id string) (StateMachine, error) {
	return s.State.Machine(id)
}

var getState = func(st *state.State) StateInterface {
	return stateShim{st}
}
