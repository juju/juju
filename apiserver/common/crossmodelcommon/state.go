// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelcommon

import (
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
)

// StatePool provides the subset of a state pool.
type StatePool interface {
	// Get returns a State for a given model from the pool.
	Get(modelUUID string) (Backend, func(), error)
}

var GetStatePool = func(sp *state.StatePool) StatePool {
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
	ControllerTag() names.ControllerTag
	Application(name string) (Application, error)
	ApplicationOffer(name string) (*crossmodel.ApplicationOffer, error)
	Model() (Model, error)
	ModelUUID() string
	ModelTag() names.ModelTag
	ModelsForUser(user names.UserTag) ([]UserModel, error)
	RemoteConnectionStatus(offerName string) (RemoteConnectionStatus, error)

	AddOfferUser(spec state.UserAccessSpec, offer names.ApplicationOfferTag) (permission.UserAccess, error)
	UserAccess(subject names.UserTag, target names.Tag) (permission.UserAccess, error)
	SetUserAccess(subject names.UserTag, target names.Tag, access permission.Access) (permission.UserAccess, error)
	RemoveUserAccess(subject names.UserTag, target names.Tag) error
}

var GetStateAccess = func(st *state.State) Backend {
	return &stateShim{st}
}

type stateShim struct {
	*state.State
}

// TODO(wallyworld)
func (s *stateShim) AddOfferUser(spec state.UserAccessSpec, offer names.ApplicationOfferTag) (permission.UserAccess, error) {
	return permission.UserAccess{}, errors.NewNotImplemented(nil, "work in progress")
}

func (s *stateShim) Model() (Model, error) {
	m, err := s.State.Model()
	return &modelShim{m}, err
}

func (s *stateShim) Application(name string) (Application, error) {
	app, err := s.State.Application(name)
	return &applicationShim{app}, err
}

func (s *stateShim) ApplicationOffer(name string) (*crossmodel.ApplicationOffer, error) {
	offers := state.NewApplicationOffers(s.State)
	return offers.ApplicationOffer(name)
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

var GetApplicationOffers = func(backend interface{}) crossmodel.ApplicationOffers {
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
	CharmURL() (curl *charm.URL, force bool)
	Name() string
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

func (s *stateShim) RemoteConnectionStatus(offerName string) (RemoteConnectionStatus, error) {
	status, err := s.State.RemoteConnectionStatus(offerName)
	return &remoteConnectionStatusShim{status}, err
}

type RemoteConnectionStatus interface {
	ConnectionCount() int
}

type remoteConnectionStatusShim struct {
	*state.RemoteConnectionStatus
}
