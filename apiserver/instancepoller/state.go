// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller

import (
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/status"
)

type StateMachine interface {
	state.Entity

	Id() string
	InstanceId() (instance.Id, error)
	ProviderAddresses() []network.Address
	SetProviderAddresses(...network.Address) error
	InstanceStatus() (status.StatusInfo, error)
	SetInstanceStatus(status.Status, string, map[string]interface{}) error
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
