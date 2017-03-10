// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/crossmodel"
	"github.com/juju/juju/apiserver/testing"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	jujutesting "github.com/juju/juju/juju/testing"
)

const (
	addOffersBackendCall  = "addOfferCall"
	listOffersBackendCall = "listOffersCall"
)

type baseCrossmodelSuite struct {
	// TODO(anastasiamac) mock to remove JujuConnSuite
	jujutesting.JujuConnSuite

	resources  *common.Resources
	authorizer testing.FakeAuthorizer

	api *crossmodel.API

	applicationOffers *mockApplicationOffers
}

func (s *baseCrossmodelSuite) addApplication(c *gc.C, name string) jujucrossmodel.ApplicationOffer {
	ch := s.AddTestingCharm(c, "wordpress")
	s.AddTestingService(c, name, ch)

	return jujucrossmodel.ApplicationOffer{
		ApplicationURL:         "local:/u/me/" + name,
		ApplicationName:        name,
		Endpoints:              map[string]charm.Relation{"db": {Name: "db"}},
		ApplicationDescription: ch.Meta().Description,
	}
}

func (s *baseCrossmodelSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.authorizer = testing.FakeAuthorizer{Tag: names.NewUserTag("testuser"), Controller: true}

	s.applicationOffers = &mockApplicationOffers{}

	var err error
	s.api, err = crossmodel.CreateAPI(
		s.applicationOffers, crossmodel.GetStateAccess(s.State), s.authorizer,
	)
	c.Assert(err, jc.ErrorIsNil)
}
