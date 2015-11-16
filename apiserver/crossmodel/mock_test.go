// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	jtesting "github.com/juju/testing"

	"github.com/juju/juju/model/crossmodel"
)

const (
	addOfferCall    = "addOfferCall"
	updateOfferCall = "updateOfferCall"
	listOffersCall  = "listOffersCall"
	removeOfferCall = "removeOfferCall"
)

type mockServiceDirectory struct {
	jtesting.Stub

	addOffer   func(offer crossmodel.ServiceOffer) error
	listOffers func(filters ...crossmodel.ServiceOfferFilter) ([]crossmodel.ServiceOffer, error)
}

func (m *mockServiceDirectory) AddOffer(offer crossmodel.ServiceOffer) error {
	m.AddCall(addOfferCall)
	return m.addOffer(offer)
}

func (m *mockServiceDirectory) ListOffers(filters ...crossmodel.ServiceOfferFilter) ([]crossmodel.ServiceOffer, error) {
	m.AddCall(listOffersCall)
	return m.listOffers(filters...)
}

func (m *mockServiceDirectory) UpdateOffer(offer crossmodel.ServiceOffer) error {
	m.AddCall(updateOfferCall)
	panic("not implemented")
}

func (m *mockServiceDirectory) Remove(url string) error {
	m.AddCall(removeOfferCall)
	panic("not implemented")
}
