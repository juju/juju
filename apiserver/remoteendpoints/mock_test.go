// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoteendpoints_test

import (
	"github.com/juju/errors"
	jtesting "github.com/juju/testing"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common/crossmodelcommon"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/testing"
)

const (
	listOffersCall = "listOffersCall"
)

type mockApplicationOffers struct {
	jtesting.Stub
	jujucrossmodel.ApplicationOffers

	addOffer   func(offer jujucrossmodel.AddApplicationOfferArgs) (*jujucrossmodel.ApplicationOffer, error)
	listOffers func(filters ...jujucrossmodel.ApplicationOfferFilter) ([]jujucrossmodel.ApplicationOffer, error)
}

func (m *mockApplicationOffers) ListOffers(filters ...jujucrossmodel.ApplicationOfferFilter) ([]jujucrossmodel.ApplicationOffer, error) {
	m.AddCall(listOffersCall)
	return m.listOffers(filters...)
}

type mockModel struct {
	uuid  string
	name  string
	owner string
}

func (m *mockModel) UUID() string {
	return m.uuid
}

func (m *mockModel) Name() string {
	return m.name
}

func (m *mockModel) Owner() names.UserTag {
	return names.NewUserTag(m.owner)
}

type mockUserModel struct {
	model crossmodelcommon.Model
}

func (m *mockUserModel) Model() crossmodelcommon.Model {
	return m.model
}

type mockCharm struct {
	meta *charm.Meta
}

func (m *mockCharm) Meta() *charm.Meta {
	return m.meta
}

type mockApplication struct {
	name  string
	charm *mockCharm
	curl  *charm.URL
}

func (m *mockApplication) Name() string {
	return m.name
}

func (m *mockApplication) Charm() (crossmodelcommon.Charm, bool, error) {
	return m.charm, true, nil
}

func (m *mockApplication) CharmURL() (curl *charm.URL, force bool) {
	return m.curl, true
}

type mockConnectionStatus struct {
	count int
}

func (m *mockConnectionStatus) ConnectionCount() int {
	return m.count
}

type mockState struct {
	crossmodelcommon.Backend
	modelUUID    string
	model        crossmodelcommon.Model
	allmodels    []crossmodelcommon.Model
	applications map[string]crossmodelcommon.Application
	connStatus   crossmodelcommon.RemoteConnectionStatus
	offerAccess  map[names.ApplicationOfferTag]permission.Access
}

func (m *mockState) Application(name string) (crossmodelcommon.Application, error) {
	app, ok := m.applications[name]
	if !ok {
		return nil, errors.NotFoundf("application %q", name)
	}
	return app, nil
}

func (m *mockState) Model() (crossmodelcommon.Model, error) {
	return m.model, nil
}

func (m *mockState) ModelUUID() string {
	return m.modelUUID
}

func (m *mockState) ModelTag() names.ModelTag {
	return names.NewModelTag(m.modelUUID)
}

func (m *mockState) ControllerTag() names.ControllerTag {
	return testing.ControllerTag
}

func (m *mockState) AllModels() ([]crossmodelcommon.Model, error) {
	if len(m.allmodels) > 0 {
		return m.allmodels, nil
	}
	return []crossmodelcommon.Model{m.model}, nil
}

func (m *mockState) GetOfferAccess(offer names.ApplicationOfferTag, user names.UserTag) (permission.Access, error) {
	access, ok := m.offerAccess[offer]
	if !ok {
		return permission.NoAccess, errors.NotFoundf("access for %q", offer)
	}
	return access, nil
}

func (m *mockState) RemoteConnectionStatus(offerName string) (crossmodelcommon.RemoteConnectionStatus, error) {
	return m.connStatus, nil
}

type mockStatePool struct {
	st map[string]crossmodelcommon.Backend
}

func (st *mockStatePool) Get(modelUUID string) (crossmodelcommon.Backend, func(), error) {
	backend, ok := st.st[modelUUID]
	if !ok {
		return nil, nil, errors.NotFoundf("model for uuid %s", modelUUID)
	}
	return backend, func() {}, nil
}
