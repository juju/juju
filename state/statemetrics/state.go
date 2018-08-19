// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package statemetrics

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/core/status"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
)

// State represents the global state managed by the Juju controller.
type State interface {
	AllMachines() ([]Machine, error)
	AllModelUUIDs() ([]string, error)
	AllUsers() ([]User, error)
	ControllerTag() names.ControllerTag
	UserAccess(names.UserTag, names.Tag) (permission.UserAccess, error)
}

// PooledState is a wrapper for State that includes methods to negotiate with
// the pool that supplied it.
type PooledState interface {
	state.PoolHelper
	State
}

// StatePool represents a pool of State objects.
type StatePool interface {
	SystemState() State
	Get(modelUUID string) (PooledState, error)
	GetModel(modelUUID string) (Model, state.PoolHelper, error)
}

// Machine represents a machine in a Juju model.
type Machine interface {
	InstanceStatus() (status.StatusInfo, error)
	Life() state.Life
	Status() (status.StatusInfo, error)
}

// Model represents a Juju model.
type Model interface {
	Life() state.Life
	ModelTag() names.ModelTag
	Status() (status.StatusInfo, error)
}

// User represents a user known to the Juju controller.
type User interface {
	IsDeleted() bool
	IsDisabled() bool
	UserTag() names.UserTag
}

type statePoolShim struct {
	pool *state.StatePool
}

// NewStatePool takes a *state.StatePool, and returns
// a StatePool value backed by it.
func NewStatePool(pool *state.StatePool) StatePool {
	return statePoolShim{pool}
}

type stateShim struct {
	*state.State
}

type pooledStateShim struct {
	*state.PooledState
}

func (p statePoolShim) SystemState() State {
	return stateShim{p.pool.SystemState()}
}

func (p statePoolShim) Get(modelUUID string) (PooledState, error) {
	st, err := p.pool.Get(modelUUID)
	if err != nil {
		return nil, err
	}
	return pooledStateShim{st}, nil
}

func (p statePoolShim) GetModel(modelUUID string) (Model, state.PoolHelper, error) {
	model, ph, err := p.pool.GetModel(modelUUID)
	if err != nil {
		return nil, nil, err
	}
	return model, ph, err
}

func (s stateShim) AllMachines() ([]Machine, error) {
	return allMachines(s.State)
}

func (s pooledStateShim) AllMachines() ([]Machine, error) {
	return allMachines(s.State)
}

func allMachines(st *state.State) ([]Machine, error) {
	machines, err := st.AllMachines()
	if err != nil {
		return nil, errors.Trace(err)
	}
	out := make([]Machine, len(machines))
	for i, m := range machines {
		if m != nil {
			out[i] = m
		}
	}
	return out, nil
}

func (s stateShim) AllUsers() ([]User, error) {
	return allUsers(s.State)
}

func (s pooledStateShim) AllUsers() ([]User, error) {
	return allUsers(s.State)
}

func allUsers(st *state.State) ([]User, error) {
	users, err := st.AllUsers(true)
	if err != nil {
		return nil, errors.Trace(err)
	}
	out := make([]User, len(users))
	for i, u := range users {
		if u != nil {
			out[i] = u
		}
	}
	return out, nil
}
