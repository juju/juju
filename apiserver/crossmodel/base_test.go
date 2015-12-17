// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"github.com/juju/names"
	jtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/crossmodel"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	jujucrossmodel "github.com/juju/juju/model/crossmodel"
)

const (
	addOfferBackendCall            = "AddOffer"
	listOfferedServicesBackendCall = "ListOfferedServices"
	listDirectoryOffersBackendCall = "ListDirectoryOffers"
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

func (s *baseCrossmodelSuite) addService(c *gc.C, name string) jujucrossmodel.OfferedService {
	ch := s.AddTestingCharm(c, "wordpress")
	s.AddTestingService(c, name, ch)

	return jujucrossmodel.OfferedService{
		ServiceURL:  "local:/u/me/" + name,
		ServiceName: name,
		CharmName:   ch.Meta().Name,
		Endpoints:   map[string]string{"db": "db"},
		Description: ch.Meta().Description,
	}
}

func (s *baseCrossmodelSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.authorizer = testing.FakeAuthorizer{names.NewUserTag("testuser"), true}

	s.serviceBackend = &mockServiceBackend{}

	var err error
	s.api, err = crossmodel.CreateAPI(s.serviceBackend, crossmodel.GetStateAccess(s.State), s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
}

type mockServiceBackend struct {
	jtesting.Stub

	addOffer            func(offer jujucrossmodel.OfferedService, offerParams params.AddServiceOffer) error
	listOfferedServices func(filter ...jujucrossmodel.OfferedServiceFilter) ([]jujucrossmodel.OfferedService, error)
	listDirectoryOffers func(filter params.OfferFilters) (params.ServiceOfferResults, error)
}

func (m *mockServiceBackend) AddOffer(offer jujucrossmodel.OfferedService, offerParams params.AddServiceOffer) error {
	m.MethodCall(m, addOfferBackendCall, offer)
	return m.addOffer(offer, offerParams)
}

func (m *mockServiceBackend) ListOfferedServices(filter ...jujucrossmodel.OfferedServiceFilter) ([]jujucrossmodel.OfferedService, error) {
	m.MethodCall(m, listOfferedServicesBackendCall, filter)
	return m.listOfferedServices(filter...)
}

func (m *mockServiceBackend) ListDirectoryOffers(filter params.OfferFilters) (params.ServiceOfferResults, error) {
	m.MethodCall(m, listDirectoryOffersBackendCall, filter)
	return m.listDirectoryOffers(filter)
}
