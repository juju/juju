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
	addOffersBackendCall  = "AddOffers"
	listOffersBackendCall = "ListOffers"
)

type baseCrossmodelSuite struct {
	// TODO(anastasiamac) mock to remove JujuConnSuite
	// This Suite is required at the moment as we cannot easily mock out a state.Service object.
	jujutesting.JujuConnSuite

	resources  *common.Resources
	authorizer testing.FakeAuthorizer

	api *crossmodel.API

	applicationDirectory             *mockApplicationOffersAPI
	makeOfferedApplicationParamsFunc func(p params.ApplicationOfferParams) (params.ApplicationOffer, error)
}

func (s *baseCrossmodelSuite) addApplication(c *gc.C, name string) jujucrossmodel.OfferedApplication {
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

	s.applicationDirectory = &mockApplicationOffersAPI{}

	var err error
	s.api, err = crossmodel.CreateAPI(
		s.applicationDirectory, crossmodel.GetStateAccess(s.State), s.authorizer,
		func(p params.ApplicationOfferParams) (params.ApplicationOffer, error) {
			if s.makeOfferedApplicationParamsFunc != nil {
				return s.makeOfferedApplicationParamsFunc(p)
			}
			return crossmodel.MakeOfferedApplicationParams(s.api, p)
		})
	c.Assert(err, jc.ErrorIsNil)
}

type mockApplicationOffersAPI struct {
	jtesting.Stub

	addOffers  func(offers params.AddApplicationOffers) (params.ErrorResults, error)
	listOffers func(filters params.OfferFilters) (params.ApplicationOfferResults, error)
}

func (m *mockApplicationOffersAPI) AddOffers(offers params.AddApplicationOffers) (params.ErrorResults, error) {
	m.MethodCall(m, addOffersBackendCall)
	return m.addOffers(offers)
}

func (m *mockApplicationOffersAPI) ListOffers(filters params.OfferFilters) (params.ApplicationOfferResults, error) {
	m.MethodCall(m, listOffersBackendCall, filters)
	return m.listOffers(filters)
}
