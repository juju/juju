// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller

import (
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/state"
)

// StateMachine represents a machine from state package.
type StateMachine interface {
	state.Entity

	ProviderAddresses() network.SpaceAddresses
	SetProviderAddresses(controller.Config, ...network.SpaceAddress) error
	String() string
	Refresh() error
	Life() state.Life
	Id() string
}

type StateInterface interface {
	state.ModelMachinesWatcher
	state.EntityFinder

	Machine(id string) (StateMachine, error)
}

type machineShim struct {
	*state.Machine
}

// TODO - CAAS(ericclaudejones): This should contain state alone, model will be
// removed once all relevant methods are moved from state to model.
type stateShim struct {
	*state.State
	*state.Model
}

func (s stateShim) Machine(id string) (StateMachine, error) {
	m, err := s.State.Machine(id)
	if err != nil {
		return nil, err
	}

	return machineShim{Machine: m}, nil
}

var getState = func(st *state.State, m *state.Model) StateInterface {
	return stateShim{State: st, Model: m}
}
