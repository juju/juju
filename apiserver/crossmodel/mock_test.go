// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"errors"

	jtesting "github.com/juju/testing"

	"github.com/juju/juju/model/crossmodel"
	"github.com/juju/juju/state"
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

type mockState struct {
	watchOfferedServices func() state.StringsWatcher
}

func (m *mockState) WatchOfferedServices() state.StringsWatcher {
	return m.watchOfferedServices()
}

func (m *mockState) Service(name string) (service *state.Service, err error) {
	return nil, errors.New("not implemented")
}

func (m *mockState) EnvironUUID() string {
	return "uuid"
}

type mockStringsWatcher struct {
	state.StringsWatcher
	changes chan []string
}

func (m *mockStringsWatcher) Changes() <-chan []string {
	return m.changes
}

func (m *mockStringsWatcher) Stop() error {
	return nil
}

type mockOfferLister struct {
	listOffers func(filters ...crossmodel.OfferedServiceFilter) ([]crossmodel.OfferedService, error)
}

func (m *mockOfferLister) ListOffers(filters ...crossmodel.OfferedServiceFilter) ([]crossmodel.OfferedService, error) {
	return m.listOffers(filters...)
}
