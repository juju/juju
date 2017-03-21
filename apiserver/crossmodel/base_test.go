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
)

const (
	addOffersBackendCall  = "addOfferCall"
	listOffersBackendCall = "listOffersCall"
)

type baseCrossmodelSuite struct {
	resources  *common.Resources
	authorizer testing.FakeAuthorizer

	api *crossmodel.API

	mockState         *mockState
	mockStatePool     *mockStatePool
	applicationOffers *mockApplicationOffers
}

func (s *baseCrossmodelSuite) addApplication(c *gc.C, name string) jujucrossmodel.ApplicationOffer {
	return jujucrossmodel.ApplicationOffer{
		OfferName:              "offer-" + name,
		ApplicationName:        name,
		Endpoints:              map[string]charm.Relation{"db": {Name: "db"}},
		ApplicationDescription: "applicaion description",
	}
}

func (s *baseCrossmodelSuite) SetUpTest(c *gc.C) {
	s.resources = common.NewResources()
	s.authorizer = testing.FakeAuthorizer{Tag: names.NewUserTag("testuser"), Controller: true}

	s.applicationOffers = &mockApplicationOffers{}

	getApplicationOffers := func(interface{}) jujucrossmodel.ApplicationOffers {
		return s.applicationOffers
	}

	var err error
	s.mockState = &mockState{modelUUID: "uuid"}
	s.mockStatePool = &mockStatePool{map[string]crossmodel.Backend{s.mockState.modelUUID: s.mockState}}
	s.api, err = crossmodel.CreateAPI(
		getApplicationOffers, s.mockState, s.mockStatePool, s.authorizer,
	)
	c.Assert(err, jc.ErrorIsNil)
}
