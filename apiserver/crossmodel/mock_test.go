// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"errors"

	jtesting "github.com/juju/testing"

	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/state"
)

const (
	addOfferCall    = "addOfferCall"
	updateOfferCall = "updateOfferCall"
	listOffersCall  = "listOffersCall"
	removeOfferCall = "removeOfferCall"
)

type mockApplicationOffers struct {
	jtesting.Stub

	addOffer   func(offer crossmodel.AddApplicationOfferArgs) (*crossmodel.ApplicationOffer, error)
	listOffers func(filters ...crossmodel.ApplicationOfferFilter) ([]crossmodel.ApplicationOffer, error)
}

func (m *mockApplicationOffers) AddOffer(offer crossmodel.AddApplicationOfferArgs) (*crossmodel.ApplicationOffer, error) {
	m.AddCall(addOfferCall)
	return m.addOffer(offer)
}

func (m *mockApplicationOffers) ListOffers(filters ...crossmodel.ApplicationOfferFilter) ([]crossmodel.ApplicationOffer, error) {
	m.AddCall(listOffersCall)
	return m.listOffers(filters...)
}

func (m *mockApplicationOffers) UpdateOffer(offer crossmodel.AddApplicationOfferArgs) (*crossmodel.ApplicationOffer, error) {
	m.AddCall(updateOfferCall)
	panic("not implemented")
}

func (m *mockApplicationOffers) Remove(url string) error {
	m.AddCall(removeOfferCall)
	panic("not implemented")
}

type mockState struct{}

func (m *mockState) Application(name string) (application *state.Application, err error) {
	return nil, errors.New("not implemented")
}
