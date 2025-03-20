// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewall

import (
	"github.com/juju/errors"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/relation"
	"github.com/juju/juju/state"
)

// State provides the subset of global state required by the
// remote firewaller facade.
type State interface {
	state.ModelMachinesWatcher

	KeyRelation(string) (Relation, error)

	Unit(string) (Unit, error)

	Machine(string) (Machine, error)

	Application(string) (Application, error)
}

// TODO(wallyworld) - for tests, remove when remaining firewaller tests become unit tests.
func StateShim(st *state.State, m *state.Model) stateShim {
	return stateShim{st, m}
}

// TODO - CAAS(ericclaudejones): This should contain state alone, model will be
// removed once all relevant methods are moved from state to model.
type stateShim struct {
	*state.State
	*state.Model
}

func (st stateShim) KeyRelation(key string) (Relation, error) {
	rel, err := st.State.KeyRelation(key)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return relationShim{rel}, nil
}

type Relation interface {
	status.StatusSetter
	Endpoints() []relation.Endpoint
	WatchUnits(applicationName string) (state.RelationUnitsWatcher, error)
}

type relationShim struct {
	*state.Relation
}

func (st stateShim) Application(name string) (Application, error) {
	app, err := st.State.Application(name)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return applicationShim{app}, nil
}

type Application interface {
	Name() string
}

type applicationShim struct {
	*state.Application
}

type Unit interface {
	Name() string
	PublicAddress() (network.SpaceAddress, error)
	AssignedMachineId() (string, error)
}

func (st stateShim) Unit(name string) (Unit, error) {
	return st.State.Unit(name)
}

type Machine interface {
	Id() string
	WatchAddresses() state.NotifyWatcher
	IsManual() (bool, error)
}

func (st stateShim) Machine(id string) (Machine, error) {
	return st.State.Machine(id)
}
