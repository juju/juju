// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers

import (
	"github.com/juju/errors"

	commoncrossmodel "github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/core/crossmodel"
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
	st, err := pool.StatePool.Get(modelUUID)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	return &stateShim{
		st:      st.State,
		Backend: commoncrossmodel.GetBackend(st.State),
	}, func() { st.Release() }, nil
}

// Backend provides selected methods off the state.State struct.
type Backend interface {
	commoncrossmodel.Backend
	ApplicationOffer(name string) (*crossmodel.ApplicationOffer, error)
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

func (s *stateShim) ApplicationOffer(name string) (*crossmodel.ApplicationOffer, error) {
	return nil, errors.NotImplementedf("cross model relations are disabled until " +
		"backend functionality is moved to domain")
}

func (s *stateShim) OfferConnections(offerUUID string) ([]OfferConnection, error) {
	// todo(gfouillet): cross model relations are disabled until backend
	//   functionality is moved to domain, so we just return an empty list until it is done
	return nil, nil
}

type OfferConnection interface {
	SourceModelUUID() string
	UserName() string
	RelationKey() string
	RelationId() int
}

type applicationOfferShim struct {
}

func (a applicationOfferShim) AddOffer(offer crossmodel.AddApplicationOfferArgs) (*crossmodel.ApplicationOffer, error) {
	return nil, errors.NotImplementedf("cross model relations are disabled until " +
		"backend functionality is moved to domain")
}

func (a applicationOfferShim) UpdateOffer(offer crossmodel.AddApplicationOfferArgs) (*crossmodel.ApplicationOffer, error) {
	return nil, errors.NotImplementedf("cross model relations are disabled until " +
		"backend functionality is moved to domain")
}

func (a applicationOfferShim) ApplicationOffer(offerName string) (*crossmodel.ApplicationOffer, error) {
	return nil, errors.NotImplementedf("cross model relations are disabled until " +
		"backend functionality is moved to domain")
}

func (a applicationOfferShim) ApplicationOfferForUUID(offerUUID string) (*crossmodel.ApplicationOffer, error) {
	return nil, errors.NotImplementedf("cross model relations are disabled until " +
		"backend functionality is moved to domain")
}

func (a applicationOfferShim) ListOffers(filter ...crossmodel.ApplicationOfferFilter) ([]crossmodel.ApplicationOffer, error) {
	// todo(gfouillet): cross model relations are disabled until backend
	//   functionality is moved to domain, so we just return an empty list until it is done
	return nil, nil
}

func (a applicationOfferShim) Remove(offerName string, force bool) error {
	return errors.NotImplementedf("cross model relations are disabled until " +
		"backend functionality is moved to domain")
}

func (a applicationOfferShim) AllApplicationOffers() (offers []*crossmodel.ApplicationOffer, _ error) {
	// todo(gfouillet): cross model relations are disabled until backend
	//   functionality is moved to domain, so we just return an empty list until it is done
	return nil, nil
}

func GetApplicationOffers(i interface{}) crossmodel.ApplicationOffers {
	return applicationOfferShim{}
}
