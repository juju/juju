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
}

type stateShim struct {
	common.Backend
	st *state.State
}

func (st stateShim) ListOffers(filter ...crossmodel.ApplicationOfferFilter) ([]crossmodel.ApplicationOffer, error) {
	oa := state.NewApplicationOffers(st.st)
	return oa.ListOffers(filter...)
}

type Model interface {
	Name() string
	Owner() names.UserTag
}

func (st stateShim) Model() (Model, error) {
	return st.st.Model()
}
