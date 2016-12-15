// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"errors"

	jtesting "github.com/juju/testing"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/state"
)

const (
	addOfferCall    = "addOfferCall"
	updateOfferCall = "updateOfferCall"
	listOffersCall  = "listOffersCall"
	removeOfferCall = "removeOfferCall"
)

type mockApplicationDirectory struct {
	jtesting.Stub

	addOffer   func(offer crossmodel.ApplicationOffer) error
	listOffers func(filters ...crossmodel.ApplicationOfferFilter) ([]crossmodel.ApplicationOffer, error)
}

func (m *mockApplicationDirectory) AddOffer(offer crossmodel.ApplicationOffer) error {
	m.AddCall(addOfferCall)
	return m.addOffer(offer)
}

func (m *mockApplicationDirectory) ListOffers(filters ...crossmodel.ApplicationOfferFilter) ([]crossmodel.ApplicationOffer, error) {
	m.AddCall(listOffersCall)
	return m.listOffers(filters...)
}

func (m *mockApplicationDirectory) UpdateOffer(offer crossmodel.ApplicationOffer) error {
	m.AddCall(updateOfferCall)
	panic("not implemented")
}

func (m *mockApplicationDirectory) Remove(url string) error {
	m.AddCall(removeOfferCall)
	panic("not implemented")
}

type mockState struct {
	watchOfferedApplications func() state.StringsWatcher
}

func (m *mockState) WatchOfferedApplications() state.StringsWatcher {
	return m.watchOfferedApplications()
}

func (m *mockState) ForModel(tag names.ModelTag) (*state.State, error) {
	return nil, errors.New("not implemented")
}

func (m *mockState) Application(name string) (application *state.Application, err error) {
	return nil, errors.New("not implemented")
}

func (m *mockState) ModelUUID() string {
	return "uuid"
}

func (m *mockState) ModelTag() names.ModelTag {
	return names.NewModelTag("uuid")
}

func (m *mockState) ModelName() (string, error) {
	return "prod", nil
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
	listOffers func(filters ...crossmodel.OfferedApplicationFilter) ([]crossmodel.OfferedApplication, error)
}

func (m *mockOfferLister) ListOffers(filters ...crossmodel.OfferedApplicationFilter) ([]crossmodel.OfferedApplication, error) {
	return m.listOffers(filters...)
}
