// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package statemetrics

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
	"github.com/juju/juju/status"
)

// State represents the global state managed by the Juju controller.
type State interface {
	AllMachines() ([]Machine, error)
	AllModels() ([]Model, error)
	AllUsers() ([]User, error)
	ControllerTag() names.ControllerTag
	ForModel(names.ModelTag) (StateCloser, error)
	UserAccess(names.UserTag, names.Tag) (permission.UserAccess, error)
}

// StateCloser extends the State interface with a Close method.
type StateCloser interface {
	State
	Close() error
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

// NewState takes a *state.State, and returns a State value backed by it.
func NewState(st *state.State) State {
	return stateShim{st}
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

func (s stateShim) AllModels() ([]Model, error) {
	models, err := s.State.AllModels()
	if err != nil {
		return nil, errors.Trace(err)
	}
	out := make([]Model, len(models))
	for i, m := range models {
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

func (s stateShim) ForModel(tag names.ModelTag) (StateCloser, error) {
	st, err := s.State.ForModel(tag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return stateShim{st}, nil
}
