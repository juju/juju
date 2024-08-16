// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers

import (
	"github.com/juju/errors"
	"github.com/juju/names/v5"

	commoncrossmodel "github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/state"
)

// StatePool provides the subset of a state pool.
type StatePool interface {
	// Get returns a State for a given model from the pool.
	Get(modelUUID string) (Backend, func(), error)

	// Get returns a Model from the pool.
	GetModel(modelUUID string) (Model, func(), error)
}

var GetStatePool = func(sp *state.StatePool) StatePool {
	return &statePoolShim{sp}

}

type statePoolShim struct {
	*state.StatePool
}

func (pool statePoolShim) Get(modelUUID string) (Backend, func(), error) {
	st, err := pool.StatePool.Get(modelUUID)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	return &stateShim{
		st:      st.State,
		Backend: commoncrossmodel.GetBackend(st.State),
	}, func() { st.Release() }, nil
}

func (pool statePoolShim) GetModel(modelUUID string) (Model, func(), error) {
	m, ph, err := pool.StatePool.GetModel(modelUUID)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	return &modelShim{m}, func() { ph.Release() }, nil
}

// Backend provides selected methods off the state.State struct.
type Backend interface {
	commoncrossmodel.Backend
	ApplicationOffer(name string) (*crossmodel.ApplicationOffer, error)
	Model() (Model, error)
	OfferConnections(string) ([]OfferConnection, error)
}

var GetStateAccess = func(st *state.State) Backend {
	return &stateShim{
		st:      st,
		Backend: commoncrossmodel.GetBackend(st),
	}
}

type stateShim struct {
	commoncrossmodel.Backend
	st *state.State
}

func (s *stateShim) Model() (Model, error) {
	m, err := s.st.Model()
	return &modelShim{m}, err
}

func (s *stateShim) ApplicationOffer(name string) (*crossmodel.ApplicationOffer, error) {
	offers := state.NewApplicationOffers(s.st)
	return offers.ApplicationOffer(name)
}

var GetApplicationOffers = func(backend interface{}) crossmodel.ApplicationOffers {
	switch st := backend.(type) {
	case *state.State:
		return state.NewApplicationOffers(st)
	case *stateShim:
		return state.NewApplicationOffers(st.st)
	}
	return nil
}

type Model interface {
	UUID() string
	ModelTag() names.ModelTag
	Name() string
	Type() state.ModelType
	Owner() names.UserTag
}

type modelShim struct {
	*state.Model
}

func (s *stateShim) OfferConnections(offerUUID string) ([]OfferConnection, error) {
	conns, err := s.st.OfferConnections(offerUUID)
	if err != nil {
		return nil, err
	}
	result := make([]OfferConnection, len(conns))
	for i, oc := range conns {
		result[i] = offerConnectionShim{oc}
	}
	return result, nil
}

type OfferConnection interface {
	SourceModelUUID() string
	UserName() string
	RelationKey() string
	RelationId() int
}

type offerConnectionShim struct {
	*state.OfferConnection
}
