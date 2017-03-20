// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/state"
)

// StatePool provides the subset of a state pool required by the
// crossmodel facade.
type StatePool interface {
	// Get returns a State for a given model from the pool.
	Get(modelUUID string) (Backend, func(), error)
}

var getStatePool = func(sp *state.StatePool) StatePool {
	return &statePoolShim{sp}

}

type statePoolShim struct {
	*state.StatePool
}

func (pool statePoolShim) Get(modelUUID string) (Backend, func(), error) {
	st, closer, err := pool.StatePool.Get(modelUUID)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	return &stateShim{st}, closer, nil
}

// Backend provides selected methods off the state.State struct.
type Backend interface {
	Application(name string) (Application, error)
	Model() (Model, error)
	ModelUUID() string
	ModelsForUser(user names.UserTag) ([]UserModel, error)
}

var getStateAccess = func(st *state.State) Backend {
	return &stateShim{st}
}

type stateShim struct {
	*state.State
}

func (s *stateShim) Model() (Model, error) {
	m, err := s.State.Model()
	return &modelShim{m}, err
}

func (s *stateShim) Application(name string) (Application, error) {
	app, err := s.State.Application(name)
	return &applicationShim{app}, err
}

func (s *stateShim) ModelsForUser(user names.UserTag) ([]UserModel, error) {
	usermodels, err := s.State.ModelsForUser(user)
	if err != nil {
		return nil, err
	}
	var result []UserModel
	for _, um := range usermodels {
		result = append(result, &userModelShim{um})
	}
	return result, err
}

var getApplicationOffers = func(backend interface{}) crossmodel.ApplicationOffers {
	switch st := backend.(type) {
	case *state.State:
		return state.NewApplicationOffers(st)
	case *stateShim:
		return state.NewApplicationOffers(st.State)
	}
	return nil
}

type Application interface {
	Charm() (ch Charm, force bool, err error)
}

type applicationShim struct {
	*state.Application
}

func (a *applicationShim) Charm() (ch Charm, force bool, err error) {
	return a.Application.Charm()
}

type Charm interface {
	Meta() *charm.Meta
}

type Model interface {
	UUID() string
	Name() string
	Owner() names.UserTag
}

type modelShim struct {
	*state.Model
}

type UserModel interface {
	Model() Model
}

type userModelShim struct {
	*state.UserModel
}

func (um *userModelShim) Model() Model {
	return &modelShim{um.UserModel.Model}
}
