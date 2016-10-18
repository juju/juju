// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	jtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/crossmodel"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/testing"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	jujutesting "github.com/juju/juju/juju/testing"
)

const (
	addOfferBackendCall                = "AddOffer"
	listOfferedApplicationsBackendCall = "ListOfferedApplications"
	listDirectoryOffersBackendCall     = "ListDirectoryOffers"
)

type baseCrossmodelSuite struct {
	// TODO(anastasiamac) mock to remove JujuConnSuite
	// This Suite is required at the moment as we cannot easily mock out a state.Service object.
	jujutesting.JujuConnSuite

	resources  *common.Resources
	authorizer testing.FakeAuthorizer

	api *crossmodel.API

	serviceBackend *mockServiceBackend
}

func (s *baseCrossmodelSuite) addService(c *gc.C, name string) jujucrossmodel.OfferedApplication {
	ch := s.AddTestingCharm(c, "wordpress")
	s.AddTestingService(c, name, ch)

	return jujucrossmodel.OfferedApplication{
		ApplicationURL:  "local:/u/me/" + name,
		ApplicationName: name,
		CharmName:       ch.Meta().Name,
		Endpoints:       map[string]string{"db": "db"},
		Description:     ch.Meta().Description,
	}
}

func (s *baseCrossmodelSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.authorizer = testing.FakeAuthorizer{Tag: names.NewUserTag("testuser"), EnvironManager: true}

	s.serviceBackend = &mockServiceBackend{}

	var err error
	s.api, err = crossmodel.CreateAPI(s.serviceBackend, crossmodel.GetStateAccess(s.State), s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
}

type mockServiceBackend struct {
	jtesting.Stub

	addOffer                func(offer jujucrossmodel.OfferedApplication, offerParams params.AddApplicationOffer) error
	listOfferedApplications func(filter ...jujucrossmodel.OfferedApplicationFilter) ([]jujucrossmodel.OfferedApplication, error)
	listDirectoryOffers     func(filter params.OfferFilters) (params.ApplicationOfferResults, error)
}

func (m *mockServiceBackend) AddOffer(offer jujucrossmodel.OfferedApplication, offerParams params.AddApplicationOffer) error {
	m.MethodCall(m, addOfferBackendCall, offer)
	return m.addOffer(offer, offerParams)
}

func (m *mockServiceBackend) ListOfferedApplications(filter ...jujucrossmodel.OfferedApplicationFilter) ([]jujucrossmodel.OfferedApplication, error) {
	m.MethodCall(m, listOfferedApplicationsBackendCall, filter)
	return m.listOfferedApplications(filter...)
}

func (m *mockServiceBackend) ListDirectoryOffers(filter params.OfferFilters) (params.ApplicationOfferResults, error) {
	m.MethodCall(m, listDirectoryOffersBackendCall, filter)
	return m.listDirectoryOffers(filter)
}
