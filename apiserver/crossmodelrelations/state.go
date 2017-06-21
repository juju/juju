// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelations

import (
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
}

type stateShim struct {
	common.Backend
	st *state.State
}

func (st stateShim) ListOffers(filter ...crossmodel.ApplicationOfferFilter) ([]crossmodel.ApplicationOffer, error) {
	oa := state.NewApplicationOffers(st.st)
	return oa.ListOffers(filter...)
}
