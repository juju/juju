// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	apicrossmodel "github.com/juju/juju/api/crossmodel"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/crossmodel"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/testing"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/feature"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&applicationdirectorySuite{})

type applicationdirectorySuite struct {
	coretesting.BaseSuite

	resources  *common.Resources
	authoriser testing.FakeAuthorizer

	api                  crossmodel.ApplicationOffersAPI
	calls                []string
	applicationdirectory *mockApplicationDirectory
	offers               map[string]jujucrossmodel.ApplicationOffer
}

func (s *applicationdirectorySuite) constructApplicationDirectory() *mockApplicationDirectory {
	return &mockApplicationDirectory{
		addOffer: func(offer jujucrossmodel.ApplicationOffer) error {
			s.calls = append(s.calls, "addoffer")
			s.offers[offer.ApplicationURL] = offer
			return nil
		},
		listOffers: func(filters ...jujucrossmodel.ApplicationOfferFilter) ([]jujucrossmodel.ApplicationOffer, error) {
			s.calls = append(s.calls, "listoffers")
			var result []jujucrossmodel.ApplicationOffer
			for _, filter := range filters {
				if offer, ok := s.offers[filter.ApplicationURL]; ok {
					result = append(result, offer)
				}
			}
			return result, nil
		},
	}
}

func (s *applicationdirectorySuite) SetUpTest(c *gc.C) {
	s.SetInitialFeatureFlags(feature.CrossModelRelations)
	s.authoriser = testing.FakeAuthorizer{
		Tag: names.NewUserTag("testuser"),
	}
	s.resources = common.NewResources()
	s.AddCleanup(func(*gc.C) { s.resources.StopAll() })

	s.calls = []string{}
	s.offers = make(map[string]jujucrossmodel.ApplicationOffer)
	s.applicationdirectory = s.constructApplicationDirectory()

	var err error
	serviceAPIFactory, err := crossmodel.NewServiceAPIFactory(
		func() jujucrossmodel.ApplicationDirectory { return s.applicationdirectory },
		nil,
	)
	c.Assert(err, jc.ErrorIsNil)
	s.api, err = crossmodel.CreateApplicationOffersAPI(serviceAPIFactory, s.authoriser)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationdirectorySuite) TestUnauthorised(c *gc.C) {
	s.authoriser = testing.FakeAuthorizer{}
	_, err := crossmodel.CreateApplicationOffersAPI(nil, s.authoriser)
	c.Assert(err, gc.Equals, common.ErrPerm)
}

func (s *applicationdirectorySuite) assertCalls(c *gc.C, expectedCalls []string) {
	c.Assert(s.calls, jc.SameContents, expectedCalls)
}

var fakeUUID = "df136476-12e9-11e4-8a70-b2227cce2b54"

func (s *applicationdirectorySuite) TestAddOffer(c *gc.C) {
	offers := params.AddApplicationOffers{
		Offers: []params.AddApplicationOffer{
			{
				ApplicationOffer: params.ApplicationOffer{
					ApplicationURL:  "local:/u/user/servicename",
					ApplicationName: "service",
					SourceModelTag:  names.NewModelTag(fakeUUID).String(),
				},
			},
			{
				ApplicationOffer: params.ApplicationOffer{
					ApplicationURL:  "local:/u/user/anothername",
					ApplicationName: "service",
					SourceModelTag:  names.NewModelTag(fakeUUID).String(),
				},
			},
		},
	}
	results, err := s.api.AddOffers(offers)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 2)
	c.Assert(results.Results[0].Error, gc.IsNil)
	s.assertCalls(c, []string{"addoffer", "addoffer"})
	offer0, err := apicrossmodel.MakeOfferFromParams(offers.Offers[0].ApplicationOffer)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.offers["local:/u/user/servicename"], jc.DeepEquals, offer0)
	offer1, err := apicrossmodel.MakeOfferFromParams(offers.Offers[1].ApplicationOffer)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.offers["local:/u/user/anothername"], jc.DeepEquals, offer1)
}

func (s *applicationdirectorySuite) TestAddOfferError(c *gc.C) {
	s.applicationdirectory.addOffer = func(offer jujucrossmodel.ApplicationOffer) error {
		s.calls = append(s.calls, "addoffer")
		return errors.New("error")
	}
	offers := params.AddApplicationOffers{
		Offers: []params.AddApplicationOffer{
			{
				ApplicationOffer: params.ApplicationOffer{
					ApplicationURL:  "local:/u/user/servicename",
					ApplicationName: "service",
					SourceModelTag:  names.NewModelTag(fakeUUID).String(),
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

func (s *applicationdirectorySuite) TestListOffers(c *gc.C) {
	s.offers["local:/u/user/servicename"] = jujucrossmodel.ApplicationOffer{
		ApplicationURL:  "local:/u/user/servicename",
		ApplicationName: "service",
		SourceModelUUID: fakeUUID,
	}
	s.offers["local:/u/user/anothername"] = jujucrossmodel.ApplicationOffer{
		ApplicationURL:  "local:/u/user/anothername",
		ApplicationName: "anotherservice",
		SourceModelUUID: fakeUUID,
	}
	results, err := s.api.ListOffers(params.OfferFilters{
		Directory: "local",
		Filters: []params.OfferFilter{
			{
				ApplicationURL: "local:/u/user/servicename",
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
	c.Assert(results.Offers[0].ApplicationURL, gc.Equals, "local:/u/user/servicename")
	c.Assert(results.Offers[0].SourceModelTag, gc.Equals, names.NewModelTag(fakeUUID).String())
}

func (s *applicationdirectorySuite) TestListOffersError(c *gc.C) {
	s.applicationdirectory.listOffers = func(filters ...jujucrossmodel.ApplicationOfferFilter) ([]jujucrossmodel.ApplicationOffer, error) {
		s.calls = append(s.calls, "listoffers")
		return nil, errors.New("error")
	}
	result, err := s.api.ListOffers(params.OfferFilters{Directory: "local"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.ErrorMatches, "error")
	s.assertCalls(c, []string{"listoffers"})
}

func (s *applicationdirectorySuite) TestListOffersNoDirectory(c *gc.C) {
	_, err := s.api.ListOffers(params.OfferFilters{})
	c.Assert(err, gc.ErrorMatches, "application directory must be specified")
}
