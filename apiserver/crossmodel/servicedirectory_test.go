// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/crossmodel"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/testing"
	jujucrossmodel "github.com/juju/juju/model/crossmodel"
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
	offers           map[string]jujucrossmodel.ServiceOffer
}

func (s *serviceDirectorySuite) constructServiceDirectory() *mockServiceDirectory {
	return &mockServiceDirectory{
		addOffer: func(offer jujucrossmodel.ServiceOffer) error {
			s.calls = append(s.calls, "addoffer")
			s.offers[offer.ServiceURL] = offer
			return nil
		},
		listOffers: func(filters ...jujucrossmodel.OfferFilter) ([]jujucrossmodel.ServiceOffer, error) {
			s.calls = append(s.calls, "listoffers")
			var result []jujucrossmodel.ServiceOffer
			for _, filter := range filters {
				if offer, ok := s.offers[filter.ServiceURL]; ok {
					result = append(result, offer)
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
	s.offers = make(map[string]jujucrossmodel.ServiceOffer)
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
				ServiceOffer: params.ServiceOffer{
					ServiceURL:  "local:/u/user/name",
					ServiceName: "service",
				},
			},
			{
				ServiceOffer: params.ServiceOffer{
					ServiceURL:  "local:/u/user/anothername",
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
	c.Assert(s.offers["local:/u/user/name"], jc.DeepEquals, offers.Offers[0].ServiceOffer)
	c.Assert(s.offers["local:/u/user/anothername"], jc.DeepEquals, offers.Offers[1].ServiceOffer)
}

func (s *serviceDirectorySuite) TestAddOfferError(c *gc.C) {
	s.serviceDirectory.addOffer = func(offer jujucrossmodel.ServiceOffer) error {
		s.calls = append(s.calls, "addoffer")
		return errors.New("error")
	}
	offers := params.AddServiceOffers{
		Offers: []params.AddServiceOffer{
			{
				ServiceOffer: params.ServiceOffer{
					ServiceURL:  "local:/u/user/name",
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
	s.offers["local:/u/user/name"] = jujucrossmodel.ServiceOffer{
		ServiceName: "service",
	}
	s.offers["local:/u/user/anothername"] = jujucrossmodel.ServiceOffer{
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
	c.Assert(results[0], jc.DeepEquals, s.offers["local:/u/user/name"])
	c.Assert(results[0].ServiceURL, gc.Equals, "local:/u/user/name")
}

func (s *serviceDirectorySuite) TestListOffersError(c *gc.C) {
	s.serviceDirectory.listOffers = func(filters ...jujucrossmodel.OfferFilter) ([]jujucrossmodel.ServiceOffer, error) {
		s.calls = append(s.calls, "listoffers")
		return nil, errors.New("error")
	}
	_, err := s.api.ListOffers(params.OfferFilters{})
	c.Assert(err, gc.ErrorMatches, "error")
	s.assertCalls(c, []string{"listoffers"})
}
