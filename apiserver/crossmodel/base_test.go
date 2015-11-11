// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	apicrossmodel "github.com/juju/juju/apiserver/crossmodel"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/crossmodel"
	coretesting "github.com/juju/juju/testing"
)

const (
	exportOfferCall = "exportOfferCall"
	searchCall      = "searchCall"
)

type baseCrossmodelSuite struct {
	coretesting.BaseSuite

	resources  *common.Resources
	authorizer testing.FakeAuthorizer

	api *apicrossmodel.API

	exporter *mockExporter
	calls    []string
}

func (s *baseCrossmodelSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.authorizer = testing.FakeAuthorizer{names.NewUserTag("testuser"), true}

	s.calls = []string{}
	var err error
	s.initialiseExporter()
	s.api, err = apicrossmodel.CreateAPI(s.exporter, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *baseCrossmodelSuite) initialiseExporter() {
	s.exporter = &mockExporter{}
	s.exporter.exportOffer = func(offer crossmodel.Offer) error {
		s.calls = append(s.calls, exportOfferCall)
		return nil
	}
	s.exporter.search = func(filter params.EndpointsSearchFilter) ([]crossmodel.RemoteServiceEndpoints, error) {
		s.calls = append(s.calls, searchCall)
		return nil, nil
	}
}

func (s *baseCrossmodelSuite) assertCalls(c *gc.C, expectedCalls ...string) {
	c.Assert(s.calls, jc.SameContents, expectedCalls)
}

type mockExporter struct {
	// ExportOffer prepares service endpoints for consumption.
	// An actual implementation will coordinate the work:
	// validate entities exist, access the service directory, write to state etc.
	exportOffer func(offer crossmodel.Offer) error

	// Search looks through offered services and returns the ones
	// that match specified filter.
	search func(filter params.EndpointsSearchFilter) ([]crossmodel.RemoteServiceEndpoints, error)
}

func (m *mockExporter) ExportOffer(offer crossmodel.Offer) error {
	return m.exportOffer(offer)
}

func (m *mockExporter) Search(filter params.EndpointsSearchFilter) ([]crossmodel.RemoteServiceEndpoints, error) {
	return m.search(filter)
}
