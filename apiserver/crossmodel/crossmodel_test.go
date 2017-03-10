// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"fmt"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/apiserver/params"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/feature"
)

type crossmodelSuite struct {
	baseCrossmodelSuite
}

var _ = gc.Suite(&crossmodelSuite{})

func (s *crossmodelSuite) SetUpTest(c *gc.C) {
	s.SetInitialFeatureFlags(feature.CrossModelRelations)
	s.baseCrossmodelSuite.SetUpTest(c)
}

func (s *crossmodelSuite) TestOffer(c *gc.C) {
	applicationName := "test"
	s.addApplication(c, applicationName)
	one := params.AddApplicationOffer{
		ApplicationURL:  "local:/u/me/test",
		ApplicationName: applicationName,
		Endpoints:       map[string]string{"db": "db"},
	}
	all := params.AddApplicationOffers{Offers: []params.AddApplicationOffer{one}}
	s.applicationOffers.addOffer = func(offer jujucrossmodel.AddApplicationOfferArgs) (*jujucrossmodel.ApplicationOffer, error) {
		c.Assert(offer.ApplicationURL, gc.Equals, one.ApplicationURL)
		c.Assert(offer.ApplicationName, gc.Equals, one.ApplicationName)
		c.Assert(offer.ApplicationDescription, gc.Equals, "A pretty popular blog engine")
		return &jujucrossmodel.ApplicationOffer{}, nil
	}

	errs, err := s.api.Offer(all)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs.Results, gc.HasLen, len(all.Offers))
	c.Assert(errs.Results[0].Error, gc.IsNil)
	s.applicationOffers.CheckCallNames(c, addOffersBackendCall)
}

func (s *crossmodelSuite) TestOfferSomeFail(c *gc.C) {
	s.addApplication(c, "one")
	s.addApplication(c, "two")
	s.addApplication(c, "paramsfail")
	one := params.AddApplicationOffer{
		ApplicationURL:  "local:/u/me/one",
		ApplicationName: "one",
		Endpoints:       map[string]string{"db": "db"},
	}
	bad := params.AddApplicationOffer{
		ApplicationURL:  "local:/u/me/bad",
		ApplicationName: "notthere",
		Endpoints:       map[string]string{"db": "db"},
	}
	bad2 := params.AddApplicationOffer{
		ApplicationURL:  "local:/u/me/bad",
		ApplicationName: "paramsfail",
		Endpoints:       map[string]string{"db": "db"},
	}
	two := params.AddApplicationOffer{
		ApplicationURL:  "local:/u/me/two",
		ApplicationName: "two",
		Endpoints:       map[string]string{"db": "db"},
	}
	all := params.AddApplicationOffers{Offers: []params.AddApplicationOffer{one, bad, bad2, two}}
	s.applicationOffers.addOffer = func(offer jujucrossmodel.AddApplicationOfferArgs) (*jujucrossmodel.ApplicationOffer, error) {
		if offer.ApplicationName == "paramsfail" {
			return nil, errors.New("params fail")
		}
		return &jujucrossmodel.ApplicationOffer{}, nil
	}
	errs, err := s.api.Offer(all)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs.Results, gc.HasLen, len(all.Offers))
	c.Assert(errs.Results[0].Error, gc.IsNil)
	c.Assert(errs.Results[3].Error, gc.IsNil)
	c.Assert(errs.Results[1].Error, gc.ErrorMatches, `getting offered application notthere: application "notthere" not found`)
	c.Assert(errs.Results[2].Error, gc.ErrorMatches, `params fail`)
	s.applicationOffers.CheckCallNames(c, addOffersBackendCall, addOffersBackendCall, addOffersBackendCall)
}

func (s *crossmodelSuite) TestOfferError(c *gc.C) {
	applicationName := "test"
	s.addApplication(c, applicationName)
	one := params.AddApplicationOffer{
		ApplicationURL:  "local:/u/me/test",
		ApplicationName: applicationName,
		Endpoints:       map[string]string{"db": "db"},
	}
	all := params.AddApplicationOffers{Offers: []params.AddApplicationOffer{one}}

	msg := "fail"

	s.applicationOffers.addOffer = func(offer jujucrossmodel.AddApplicationOfferArgs) (*jujucrossmodel.ApplicationOffer, error) {
		return nil, errors.New(msg)
	}

	errs, err := s.api.Offer(all)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs.Results, gc.HasLen, len(all.Offers))
	c.Assert(errs.Results[0].Error, gc.ErrorMatches, fmt.Sprintf(".*%v.*", msg))
	s.applicationOffers.CheckCallNames(c, addOffersBackendCall)
}

func (s *crossmodelSuite) TestShow(c *gc.C) {
	applicationName := "test"
	url := "local:/u/fred/hosted-db2"

	filter := params.ApplicationURLs{[]string{url}}
	anOffer := jujucrossmodel.ApplicationOffer{
		ApplicationName:        applicationName,
		ApplicationDescription: "description",
		ApplicationURL:         url,
		Endpoints:              map[string]charm.Relation{"db": {Name: "db"}},
	}

	s.applicationOffers.listOffers = func(filters ...jujucrossmodel.ApplicationOfferFilter) ([]jujucrossmodel.ApplicationOffer, error) {
		return []jujucrossmodel.ApplicationOffer{anOffer}, nil
	}

	found, err := s.api.ApplicationOffers(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, jc.DeepEquals,
		params.ApplicationOffersResults{[]params.ApplicationOfferResult{
			{Result: params.ApplicationOffer{
				ApplicationName:        applicationName,
				ApplicationDescription: "description",
				ApplicationURL:         url,
				Endpoints:              []params.RemoteEndpoint{{Name: "db"}}}},
		}})
	s.applicationOffers.CheckCallNames(c, listOffersBackendCall)
}

func (s *crossmodelSuite) TestShowError(c *gc.C) {
	url := "local:/u/fred/hosted-db2"
	filter := params.ApplicationURLs{[]string{url}}
	msg := "fail"

	s.applicationOffers.listOffers = func(filters ...jujucrossmodel.ApplicationOfferFilter) ([]jujucrossmodel.ApplicationOffer, error) {
		return nil, errors.New(msg)
	}

	_, err := s.api.ApplicationOffers(filter)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf(".*%v.*", msg))
	s.applicationOffers.CheckCallNames(c, listOffersBackendCall)
}

func (s *crossmodelSuite) TestShowNotFound(c *gc.C) {
	urls := []string{"local:/u/fred/hosted-db2"}
	filter := params.ApplicationURLs{urls}

	s.applicationOffers.listOffers = func(filters ...jujucrossmodel.ApplicationOfferFilter) ([]jujucrossmodel.ApplicationOffer, error) {
		return nil, nil
	}

	found, err := s.api.ApplicationOffers(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0].Error.Error(), gc.Matches, fmt.Sprintf(`offer for remote application url %v not found`, urls[0]))
	s.applicationOffers.CheckCallNames(c, listOffersBackendCall)
}

func (s *crossmodelSuite) TestShowErrorMsgMultipleURLs(c *gc.C) {
	urls := []string{"local:/u/fred/hosted-mysql", "local:/u/fred/hosted-db2"}
	filter := params.ApplicationURLs{urls}

	s.applicationOffers.listOffers = func(filters ...jujucrossmodel.ApplicationOfferFilter) ([]jujucrossmodel.ApplicationOffer, error) {
		return nil, nil
	}

	found, err := s.api.ApplicationOffers(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 2)
	c.Assert(found.Results[0].Error.Error(), gc.Matches, fmt.Sprintf(`offer for remote application url %v not found`, urls[0]))
	c.Assert(found.Results[1].Error.Error(), gc.Matches, fmt.Sprintf(`offer for remote application url %v not found`, urls[1]))
	s.applicationOffers.CheckCallNames(c, listOffersBackendCall)
}

func (s *crossmodelSuite) TestShowFoundMultiple(c *gc.C) {
	name := "test"
	url := "local:/u/fred/hosted-db2"
	anOffer := jujucrossmodel.ApplicationOffer{
		ApplicationName:        name,
		ApplicationDescription: "description",
		ApplicationURL:         url,
		Endpoints:              map[string]charm.Relation{"db": {Name: "db"}},
	}

	name2 := "testAgain"
	url2 := "local:/u/mary/hosted-db2"
	anOffer2 := jujucrossmodel.ApplicationOffer{
		ApplicationName:        name2,
		ApplicationDescription: "description2",
		ApplicationURL:         url2,
		Endpoints:              map[string]charm.Relation{"db2": {Name: "db2"}},
	}

	filter := params.ApplicationURLs{[]string{url, url2}}

	s.applicationOffers.listOffers = func(filters ...jujucrossmodel.ApplicationOfferFilter) ([]jujucrossmodel.ApplicationOffer, error) {
		return []jujucrossmodel.ApplicationOffer{anOffer, anOffer2}, nil
	}

	found, err := s.api.ApplicationOffers(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, jc.DeepEquals, params.ApplicationOffersResults{
		[]params.ApplicationOfferResult{
			{Result: params.ApplicationOffer{
				ApplicationName:        name,
				ApplicationDescription: "description",
				ApplicationURL:         url,
				Endpoints:              []params.RemoteEndpoint{{Name: "db"}}}},
			{Result: params.ApplicationOffer{
				ApplicationName:        name2,
				ApplicationDescription: "description2",
				ApplicationURL:         url2,
				Endpoints:              []params.RemoteEndpoint{{Name: "db2"}}}},
		}})
	s.applicationOffers.CheckCallNames(c, listOffersBackendCall)
}

func (s *crossmodelSuite) TestFind(c *gc.C) {
	applicationName := "test"
	url := "local:/u/fred/hosted-db2"

	filter := params.OfferFilters{
		Filters: []params.OfferFilter{
			{
				ApplicationURL:  "local:/u/fred/hosted-db2",
				ApplicationName: "test",
			},
		},
	}
	anOffer := jujucrossmodel.ApplicationOffer{
		ApplicationName:        applicationName,
		ApplicationDescription: "description",
		ApplicationURL:         url,
		Endpoints:              map[string]charm.Relation{"db": {Name: "db2"}},
	}

	s.applicationOffers.listOffers = func(filters ...jujucrossmodel.ApplicationOfferFilter) ([]jujucrossmodel.ApplicationOffer, error) {
		c.Assert(filters, gc.HasLen, 1)
		c.Assert(filters[0], jc.DeepEquals, jujucrossmodel.ApplicationOfferFilter{
			ApplicationURL:  "local:/u/fred/hosted-db2",
			ApplicationName: "test",
		})
		return []jujucrossmodel.ApplicationOffer{anOffer}, nil
	}

	found, err := s.api.FindApplicationOffers(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, jc.DeepEquals, params.FindApplicationOffersResults{
		[]params.ApplicationOffer{
			{
				ApplicationName:        applicationName,
				ApplicationDescription: "description",
				ApplicationURL:         url,
				Endpoints:              []params.RemoteEndpoint{{Name: "db"}}}},
	})
	s.applicationOffers.CheckCallNames(c, listOffersBackendCall)
}

func (s *crossmodelSuite) TestFindError(c *gc.C) {
	filter := params.OfferFilters{
		Filters: []params.OfferFilter{
			{
				ApplicationURL:  "local:/u/fred/hosted-db2",
				ApplicationName: "test",
			},
		},
	}
	msg := "fail"

	s.applicationOffers.listOffers = func(filters ...jujucrossmodel.ApplicationOfferFilter) ([]jujucrossmodel.ApplicationOffer, error) {
		return nil, errors.New(msg)
	}

	_, err := s.api.FindApplicationOffers(filter)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf(".*%v.*", msg))
	s.applicationOffers.CheckCallNames(c, listOffersBackendCall)
}
