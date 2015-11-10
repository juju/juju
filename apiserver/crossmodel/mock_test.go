// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/params"
)

type mockServiceDirectory struct {
	addOffer   func(url string, offerDetails params.ServiceOfferDetails, users []names.UserTag) error
	listOffers func(filters ...params.OfferFilter) ([]params.ServiceOffer, error)
}

func (m *mockServiceDirectory) AddOffer(url string, offerDetails params.ServiceOfferDetails, users []names.UserTag) error {
	return m.addOffer(url, offerDetails, users)
}

func (m *mockServiceDirectory) ListOffers(filters ...params.OfferFilter) ([]params.ServiceOffer, error) {
	return m.listOffers(filters...)
}
