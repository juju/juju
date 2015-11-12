// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"github.com/juju/juju/model/crossmodel"
)

type mockServiceDirectory struct {
	addOffer   func(offer crossmodel.ServiceOffer) error
	listOffers func(filters ...crossmodel.OfferFilter) ([]crossmodel.ServiceOffer, error)
}

func (m *mockServiceDirectory) AddOffer(offer crossmodel.ServiceOffer) error {
	return m.addOffer(offer)
}

func (m *mockServiceDirectory) ListOffers(filters ...crossmodel.OfferFilter) ([]crossmodel.ServiceOffer, error) {
	return m.listOffers(filters...)
}

func (m *mockServiceDirectory) UpdateOffer(offer crossmodel.ServiceOffer) error {
	panic("not implemented")
}

func (m *mockServiceDirectory) Remove(url string) error {
	panic("not implemented")
}
