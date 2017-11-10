// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package statemetrics

import (
	"github.com/juju/errors"
	"github.com/juju/juju/permission"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/state"
	"github.com/juju/juju/status"
)

// StatePool represents a pool of State objects.
type StatePool interface {
	GetController() Controller
	Get(modelUUID string) (State, state.StatePoolReleaser, error)
	GetModel(modelUUID string) (Model, state.StatePoolReleaser, error)
}

// Controller represents the state of a Controller.
type Controller interface {
	ControllerState() State
	ControllerTag() names.ControllerTag
}

// State represents the state of a Model.
type State interface {
	AllMachines() ([]Machine, error)
	AllModelUUIDs() ([]string, error)
	AllUsers() ([]User, error)
	UserAccess(names.UserTag, names.Tag) (permission.UserAccess, error)
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

// NewStatePool takes a *state.StatePool, and returns
// a StatePool value backed by it.
func NewStatePool(pool *state.StatePool) StatePool {
	return statePoolShim{pool}
}

type statePoolShim struct {
	pool *state.StatePool
}

func (p statePoolShim) GetController() Controller {
	return controllerShim{p.pool.GetController()}
}

func (p statePoolShim) Get(modelUUID string) (State, state.StatePoolReleaser, error) {
	st, releaser, err := p.pool.Get(modelUUID)
	if err != nil {
		return nil, nil, err
	}
	return stateShim{st}, releaser, nil
}

func (p statePoolShim) GetModel(modelUUID string) (Model, state.StatePoolReleaser, error) {
	model, releaser, err := p.pool.GetModel(modelUUID)
	if err != nil {
		return nil, nil, err
	}
	return model, releaser, nil
}

type controllerShim struct {
	*state.Controller
}

func (c controllerShim) ControllerState() State {
	return stateShim{c.Controller.ControllerState()}
}

type stateShim struct {
	*state.State
}

func (s stateShim) AllMachines() ([]Machine, error) {
	machines, err := s.State.AllMachines()
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
	users, err := s.State.AllUsers(true)
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
