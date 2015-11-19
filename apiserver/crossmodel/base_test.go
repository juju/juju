// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"github.com/juju/names"
	jtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/apiserver/common"
	apicrossmodel "github.com/juju/juju/apiserver/crossmodel"
	"github.com/juju/juju/apiserver/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/model/crossmodel"
)

const (
	addOfferBackendCall           = "AddOffer"
	listOffersBackendCall         = "ListOffers"
	listRemoteServicesBackendCall = "ListRemoteServices"
)

type baseCrossmodelSuite struct {
	// TODO(anastasiamac) mock to remove JujuConnSuite
	// This Suite is required at the moment as we cannot easily mock out a state.Service object.
	jujutesting.JujuConnSuite

	resources  *common.Resources
	authorizer testing.FakeAuthorizer

	api *apicrossmodel.API

	serviceBackend *mockServiceBackend
}

func (s *baseCrossmodelSuite) addService(c *gc.C, name string) crossmodel.ServiceOffer {
	ch := s.AddTestingCharm(c, "wordpress")
	s.AddTestingService(c, name, ch)

	cfg, _ := s.State.EnvironConfig()
	return crossmodel.ServiceOffer{
		ServiceName:        name,
		ServiceDescription: ch.Meta().Description,
		SourceLabel:        cfg.Name(),
		SourceEnvUUID:      s.State.EnvironUUID(),
		Endpoints:          []charm.Relation{},
	}
}

func (s *baseCrossmodelSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.authorizer = testing.FakeAuthorizer{names.NewUserTag("testuser"), true}

	s.serviceBackend = &mockServiceBackend{}

	var err error
	s.api, err = apicrossmodel.CreateAPI(s.serviceBackend, s.State, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
}

type mockServiceBackend struct {
	jtesting.Stub

	addOffer           func(offer crossmodel.ServiceOffer) error
	listOffers         func(filters ...crossmodel.ServiceOfferFilter) ([]crossmodel.ServiceOffer, error)
	listRemoteServices func(filters ...crossmodel.RemoteServiceFilter) (map[string][]crossmodel.RemoteService, error)
}

func (m *mockServiceBackend) AddOffer(offer crossmodel.ServiceOffer) error {
	m.MethodCall(m, addOfferBackendCall, offer)
	return m.addOffer(offer)
}

func (m *mockServiceBackend) ListOffers(filters ...crossmodel.ServiceOfferFilter) ([]crossmodel.ServiceOffer, error) {
	m.MethodCall(m, listOffersBackendCall, filters)
	return m.listOffers(filters...)
}

func (m *mockServiceBackend) ListRemoteServices(filters ...crossmodel.RemoteServiceFilter) (map[string][]crossmodel.RemoteService, error) {
	m.MethodCall(m, listRemoteServicesBackendCall, filters)
	return m.listRemoteServices(filters...)
}
