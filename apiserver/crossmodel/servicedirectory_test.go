// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/crossmodel"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/testing"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&serviceDirectorySuite{})

type serviceDirectorySuite struct {
	coretesting.BaseSuite

	resources  *common.Resources
	authoriser testing.FakeAuthorizer

	api              *crossmodel.ServiceDirectoryAPI
	calls            []string
	serviceDirectory *mockServiceDirectory
	offers           map[string]params.ServiceOfferDetails
}

func (s *serviceDirectorySuite) constructServiceDirectory() *mockServiceDirectory {
	return &mockServiceDirectory{
		addOffer: func(url string, offerDetails params.ServiceOfferDetails, users []names.UserTag) error {
			s.calls = append(s.calls, "addoffer")
			s.offers[url] = offerDetails
			return nil
		},
		listOffers: func(filters ...params.OfferFilter) ([]params.ServiceOffer, error) {
			s.calls = append(s.calls, "listoffers")
			var result []params.ServiceOffer
			for _, filter := range filters {
				if offer, ok := s.offers[filter.ServiceURL]; ok {
					result = append(result, params.ServiceOffer{filter.ServiceURL, offer})
				}
			}
			return result, nil
		},
	}
}

func (s *serviceDirectorySuite) SetUpTest(c *gc.C) {
	s.authoriser = testing.FakeAuthorizer{
		EnvironManager: true,
	}
	s.resources = common.NewResources()
	s.AddCleanup(func(*gc.C) { s.resources.StopAll() })

	s.calls = []string{}
	s.offers = make(map[string]params.ServiceOfferDetails)
	s.serviceDirectory = s.constructServiceDirectory()

	var err error
	s.api, err = crossmodel.CreateServiceDirectoryAPI(s.serviceDirectory, s.resources, s.authoriser)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceDirectorySuite) assertCalls(c *gc.C, expectedCalls []string) {
	c.Assert(s.calls, jc.SameContents, expectedCalls)
}

func (s *serviceDirectorySuite) TestAddOffer(c *gc.C) {
	offers := params.AddServiceOffers{
		Offers: []params.AddServiceOffer{
			{
				ServiceURL: "local:/u/user/name",
				ServiceOfferDetails: params.ServiceOfferDetails{
					ServiceName: "service",
				},
			},
			{
				ServiceURL: "local:/u/user/anothername",
				ServiceOfferDetails: params.ServiceOfferDetails{
					ServiceName: "service",
				},
			},
		},
	}
	results, err := s.api.AddOffers(offers)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 2)
	c.Assert(results.Results[0].Error, gc.IsNil)
	s.assertCalls(c, []string{"addoffer", "addoffer"})
	c.Assert(s.offers["local:/u/user/name"], jc.DeepEquals, offers.Offers[0].ServiceOfferDetails)
	c.Assert(s.offers["local:/u/user/anothername"], jc.DeepEquals, offers.Offers[1].ServiceOfferDetails)
}

func (s *serviceDirectorySuite) TestAddOfferError(c *gc.C) {
	s.serviceDirectory.addOffer = func(url string, offerDetails params.ServiceOfferDetails, users []names.UserTag) error {
		s.calls = append(s.calls, "addoffer")
		return errors.New("error")
	}
	offers := params.AddServiceOffers{
		Offers: []params.AddServiceOffer{
			{
				ServiceURL: "local:/u/user/name",
				ServiceOfferDetails: params.ServiceOfferDetails{
					ServiceName: "service",
				},
			},
		},
	}
	results, err := s.api.AddOffers(offers)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, "error")
	s.assertCalls(c, []string{"addoffer"})
}

func (s *serviceDirectorySuite) TestListOffers(c *gc.C) {
	s.offers["local:/u/user/name"] = params.ServiceOfferDetails{
		ServiceName: "service",
	}
	s.offers["local:/u/user/anothername"] = params.ServiceOfferDetails{
		ServiceName: "anotherservice",
	}
	results, err := s.api.ListOffers(params.OfferFilters{
		Filters: []params.OfferFilter{
			{
				ServiceURL: "local:/u/user/name",
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 1)
	s.assertCalls(c, []string{"listoffers"})
	c.Assert(results[0].ServiceOfferDetails, jc.DeepEquals, s.offers["local:/u/user/name"])
	c.Assert(results[0].ServiceURL, gc.Equals, "local:/u/user/name")
}

func (s *serviceDirectorySuite) TestListOffersError(c *gc.C) {
	s.serviceDirectory.listOffers = func(filters ...params.OfferFilter) ([]params.ServiceOffer, error) {
		s.calls = append(s.calls, "listoffers")
		return nil, errors.New("error")
	}
	_, err := s.api.ListOffers(params.OfferFilters{})
	c.Assert(err, gc.ErrorMatches, "error")
	s.assertCalls(c, []string{"listoffers"})
}
