// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelations

import (
	"gopkg.in/juju/names.v2"

	common "github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/state"
)

// RemoteRelationState provides the subset of global state required by the
// remote relations facade.
type CrossModelRelationsState interface {
	common.Backend

	// ListOffers returns the application offers matching any one of the filter terms.
	ListOffers(filter ...crossmodel.ApplicationOfferFilter) ([]crossmodel.ApplicationOffer, error)

	// Model returns the model entity.
	Model() (Model, error)

	// AddOfferConnection creates a new offer connection record, which records details about a
	// relation made from a remote model to an offer in the local model.
	AddOfferConnection(state.AddOfferConnectionParams) (OfferConnection, error)

	// OfferConnectionForRelation returns the offer connection details for the given relation key.
	OfferConnectionForRelation(string) (OfferConnection, error)
}

type stateShim struct {
	common.Backend
	st *state.State
}

func (st stateShim) ListOffers(filter ...crossmodel.ApplicationOfferFilter) ([]crossmodel.ApplicationOffer, error) {
	oa := state.NewApplicationOffers(st.st)
	return oa.ListOffers(filter...)
}

func (st stateShim) AddOfferConnection(arg state.AddOfferConnectionParams) (OfferConnection, error) {
	return st.st.AddOfferConnection(arg)
}

func (st stateShim) OfferConnectionForRelation(relationKey string) (OfferConnection, error) {
	return st.st.OfferConnectionForRelation(relationKey)
}

type Model interface {
	Name() string
	Owner() names.UserTag
}

func (st stateShim) Model() (Model, error) {
	return st.st.Model()
}

type OfferConnection interface {
	OfferName() string
}
