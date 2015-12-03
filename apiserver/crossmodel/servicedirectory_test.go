// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apicrossmodel "github.com/juju/juju/api/crossmodel"
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

	api              crossmodel.ServiceOffersAPI
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
		listOffers: func(filters ...jujucrossmodel.ServiceOfferFilter) ([]jujucrossmodel.ServiceOffer, error) {
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
		Tag: names.NewUserTag("testuser"),
	}
	s.resources = common.NewResources()
	s.AddCleanup(func(*gc.C) { s.resources.StopAll() })

	s.calls = []string{}
	s.offers = make(map[string]jujucrossmodel.ServiceOffer)
	s.serviceDirectory = s.constructServiceDirectory()

	var err error
	serviceAPIFactory, err := crossmodel.NewServiceAPIFactory(
		func() jujucrossmodel.ServiceDirectory { return s.serviceDirectory },
	)
	c.Assert(err, jc.ErrorIsNil)
	s.api, err = crossmodel.CreateServiceOffersAPI(serviceAPIFactory, s.resources, s.authoriser)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceDirectorySuite) TestUnauthorised(c *gc.C) {
	s.authoriser = testing.FakeAuthorizer{}
	_, err := crossmodel.CreateServiceOffersAPI(nil, s.resources, s.authoriser)
	c.Assert(err, gc.Equals, common.ErrPerm)
}

func (s *serviceDirectorySuite) assertCalls(c *gc.C, expectedCalls []string) {
	c.Assert(s.calls, jc.SameContents, expectedCalls)
}

var fakeUUID = "df136476-12e9-11e4-8a70-b2227cce2b54"

func (s *serviceDirectorySuite) TestAddOffer(c *gc.C) {
	offers := params.AddServiceOffers{
		Offers: []params.AddServiceOffer{
			{
				ServiceOffer: params.ServiceOffer{
					ServiceURL:       "local:/u/user/servicename",
					ServiceName:      "service",
					SourceEnvironTag: names.NewEnvironTag(fakeUUID).String(),
				},
			},
			{
				ServiceOffer: params.ServiceOffer{
					ServiceURL:       "local:/u/user/anothername",
					ServiceName:      "service",
					SourceEnvironTag: names.NewEnvironTag(fakeUUID).String(),
				},
			},
		},
	}
	results, err := s.api.AddOffers(offers)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 2)
	c.Assert(results.Results[0].Error, gc.IsNil)
	s.assertCalls(c, []string{"addoffer", "addoffer"})
	offer0, err := apicrossmodel.MakeOfferFromParams(offers.Offers[0].ServiceOffer)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.offers["local:/u/user/servicename"], jc.DeepEquals, offer0)
	offer1, err := apicrossmodel.MakeOfferFromParams(offers.Offers[1].ServiceOffer)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.offers["local:/u/user/anothername"], jc.DeepEquals, offer1)
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
					ServiceURL:       "local:/u/user/servicename",
					ServiceName:      "service",
					SourceEnvironTag: names.NewEnvironTag(fakeUUID).String(),
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
	s.offers["local:/u/user/servicename"] = jujucrossmodel.ServiceOffer{
		ServiceURL:    "local:/u/user/servicename",
		ServiceName:   "service",
		SourceEnvUUID: fakeUUID,
	}
	s.offers["local:/u/user/anothername"] = jujucrossmodel.ServiceOffer{
		ServiceURL:    "local:/u/user/anothername",
		ServiceName:   "anotherservice",
		SourceEnvUUID: fakeUUID,
	}
	results, err := s.api.ListOffers(params.OfferFilters{
		Directory: "local",
		Filters: []params.OfferFilter{
			{
				ServiceURL: "local:/u/user/servicename",
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Error, gc.IsNil)
	c.Assert(results.Offers, gc.HasLen, 1)
	s.assertCalls(c, []string{"listoffers"})
	offer, err := apicrossmodel.MakeOfferFromParams(results.Offers[0])
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(offer, jc.DeepEquals, s.offers["local:/u/user/servicename"])
	c.Assert(results.Offers[0].ServiceURL, gc.Equals, "local:/u/user/servicename")
	c.Assert(results.Offers[0].SourceEnvironTag, gc.Equals, names.NewEnvironTag(fakeUUID).String())
}

func (s *serviceDirectorySuite) TestListOffersError(c *gc.C) {
	s.serviceDirectory.listOffers = func(filters ...jujucrossmodel.ServiceOfferFilter) ([]jujucrossmodel.ServiceOffer, error) {
		s.calls = append(s.calls, "listoffers")
		return nil, errors.New("error")
	}
	result, err := s.api.ListOffers(params.OfferFilters{Directory: "local"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.ErrorMatches, "error")
	s.assertCalls(c, []string{"listoffers"})
}

func (s *serviceDirectorySuite) TestListOffersNoDirectory(c *gc.C) {
	_, err := s.api.ListOffers(params.OfferFilters{})
	c.Assert(err, gc.ErrorMatches, "service directory must be specified")
}
