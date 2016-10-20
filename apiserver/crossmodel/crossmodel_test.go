// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"fmt"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/crossmodel"
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
	serviceName := "test"
	expectedOffer := s.addService(c, serviceName)
	one := params.ApplicationOfferParams{
		ApplicationURL:  "local:/u/me/test",
		ApplicationName: serviceName,
		Endpoints:       []string{"db"},
	}
	all := params.ApplicationOffersParams{[]params.ApplicationOfferParams{one}}
	expectedOfferParams := params.AddApplicationOffer{
		ApplicationOffer: params.ApplicationOffer{
			ApplicationURL:         "local:/u/me/test",
			SourceModelTag:         "model-deadbeef-0bad-400d-8000-4b1d0d06f00d",
			SourceLabel:            "controller",
			ApplicationName:        "test",
			ApplicationDescription: "A pretty popular blog engine",
			Endpoints: []params.RemoteEndpoint{
				{Name: "db",
					Role:      "requirer",
					Interface: "mysql",
					Limit:     1,
					Scope:     "global",
				},
			}}}

	s.serviceBackend.addOffer = func(offer crossmodel.OfferedApplication, offerParams params.AddApplicationOffer) error {
		c.Assert(offer, gc.DeepEquals, expectedOffer)
		c.Assert(offerParams, gc.DeepEquals, expectedOfferParams)
		return nil
	}

	errs, err := s.api.Offer(all)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs.Results, gc.HasLen, len(all.Offers))
	c.Assert(errs.Results[0].Error, gc.IsNil)
	s.serviceBackend.CheckCallNames(c, addOfferBackendCall)
}

func (s *crossmodelSuite) TestOfferError(c *gc.C) {
	serviceName := "test"
	s.addService(c, serviceName)
	one := params.ApplicationOfferParams{
		ApplicationName: serviceName,
	}
	all := params.ApplicationOffersParams{[]params.ApplicationOfferParams{one}}

	msg := "fail"

	s.serviceBackend.addOffer = func(offer crossmodel.OfferedApplication, offerParams params.AddApplicationOffer) error {
		return errors.New(msg)
	}

	errs, err := s.api.Offer(all)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs.Results, gc.HasLen, len(all.Offers))
	c.Assert(errs.Results[0].Error, gc.ErrorMatches, fmt.Sprintf(".*%v.*", msg))
	s.serviceBackend.CheckCallNames(c, addOfferBackendCall)
}

func (s *crossmodelSuite) TestShow(c *gc.C) {
	serviceName := "test"
	url := "local:/u/fred/hosted-db2"

	filter := params.ApplicationURLs{[]string{url}}
	anOffer := params.ApplicationOffer{
		ApplicationName:        serviceName,
		ApplicationDescription: "description",
		ApplicationURL:         url,
		SourceModelTag:         "environment-",
		SourceLabel:            "label",
		Endpoints:              []params.RemoteEndpoint{{Name: "db"}},
	}

	s.serviceBackend.listDirectoryOffers = func(filter params.OfferFilters) (params.ApplicationOfferResults, error) {
		return params.ApplicationOfferResults{Offers: []params.ApplicationOffer{anOffer}}, nil
	}

	found, err := s.api.ApplicationOffers(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, gc.DeepEquals,
		params.ApplicationOffersResults{[]params.ApplicationOfferResult{
			{Result: params.ApplicationOffer{
				ApplicationName:        serviceName,
				ApplicationDescription: "description",
				ApplicationURL:         url,
				SourceModelTag:         "environment-",
				SourceLabel:            "label",
				Endpoints:              []params.RemoteEndpoint{{Name: "db"}}}},
		}})
	s.serviceBackend.CheckCallNames(c, listDirectoryOffersBackendCall)
}

func (s *crossmodelSuite) TestShowError(c *gc.C) {
	url := "local:/u/fred/hosted-db2"
	filter := params.ApplicationURLs{[]string{url}}
	msg := "fail"

	s.serviceBackend.listDirectoryOffers = func(filter params.OfferFilters) (params.ApplicationOfferResults, error) {
		return params.ApplicationOfferResults{}, errors.New(msg)
	}

	found, err := s.api.ApplicationOffers(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0].Error, gc.ErrorMatches, fmt.Sprintf(".*%v.*", msg))
	s.serviceBackend.CheckCallNames(c, listDirectoryOffersBackendCall)
}

func (s *crossmodelSuite) TestShowNotFound(c *gc.C) {
	urls := []string{"local:/u/fred/hosted-db2"}
	filter := params.ApplicationURLs{urls}

	s.serviceBackend.listDirectoryOffers = func(filter params.OfferFilters) (params.ApplicationOfferResults, error) {
		return params.ApplicationOfferResults{}, nil
	}

	found, err := s.api.ApplicationOffers(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0].Error.Error(), gc.Matches, fmt.Sprintf(`offer for remote application url %v not found`, urls[0]))
	s.serviceBackend.CheckCallNames(c, listDirectoryOffersBackendCall)
}

func (s *crossmodelSuite) TestShowErrorMsgMultipleURLs(c *gc.C) {
	urls := []string{"local:/u/fred/prod/foo/hosted-db2", "local:/u/fred/hosted-db2"}
	filter := params.ApplicationURLs{urls}

	s.serviceBackend.listDirectoryOffers = func(filter params.OfferFilters) (params.ApplicationOfferResults, error) {
		return params.ApplicationOfferResults{}, nil
	}

	found, err := s.api.ApplicationOffers(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 2)
	c.Assert(found.Results[0].Error.Error(), gc.Matches, fmt.Sprintf(`application URL has invalid form: %q`, urls[0]))
	c.Assert(found.Results[1].Error.Error(), gc.Matches, fmt.Sprintf(`offer for remote application url %v not found`, urls[1]))
	s.serviceBackend.CheckCallNames(c, listDirectoryOffersBackendCall)
}

func (s *crossmodelSuite) TestShowNotFoundEmpty(c *gc.C) {
	urls := []string{"local:/u/fred/hosted-db2"}
	filter := params.ApplicationURLs{urls}

	s.serviceBackend.listDirectoryOffers = func(filter params.OfferFilters) (params.ApplicationOfferResults, error) {
		return params.ApplicationOfferResults{}, nil
	}

	found, err := s.api.ApplicationOffers(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0].Error.Error(), gc.Matches, fmt.Sprintf(`offer for remote application url %v not found`, urls[0]))
	s.serviceBackend.CheckCallNames(c, listDirectoryOffersBackendCall)
}

func (s *crossmodelSuite) TestShowFoundMultiple(c *gc.C) {
	name := "test"
	url := "local:/u/fred/hosted-db2"
	anOffer := params.ApplicationOffer{
		ApplicationName:        name,
		ApplicationDescription: "description",
		ApplicationURL:         url,
		SourceModelTag:         "environment-",
		SourceLabel:            "label",
		Endpoints:              []params.RemoteEndpoint{{Name: "db"}},
	}

	name2 := "testAgain"
	url2 := "local:/u/mary/hosted-db2"
	anOffer2 := params.ApplicationOffer{
		ApplicationName:        name2,
		ApplicationDescription: "description2",
		ApplicationURL:         url2,
		SourceModelTag:         "environment-",
		SourceLabel:            "label2",
		Endpoints:              []params.RemoteEndpoint{{Name: "db2"}},
	}

	filter := params.ApplicationURLs{[]string{url, url2}}

	s.serviceBackend.listDirectoryOffers = func(filter params.OfferFilters) (params.ApplicationOfferResults, error) {
		return params.ApplicationOfferResults{Offers: []params.ApplicationOffer{anOffer, anOffer2}}, nil
	}

	found, err := s.api.ApplicationOffers(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, gc.DeepEquals, params.ApplicationOffersResults{
		[]params.ApplicationOfferResult{
			{Result: params.ApplicationOffer{
				ApplicationName:        name,
				ApplicationDescription: "description",
				ApplicationURL:         url,
				SourceModelTag:         "environment-",
				SourceLabel:            "label",
				Endpoints:              []params.RemoteEndpoint{{Name: "db"}}}},
			{Result: params.ApplicationOffer{
				ApplicationName:        name2,
				ApplicationDescription: "description2",
				ApplicationURL:         url2,
				SourceModelTag:         "environment-",
				SourceLabel:            "label2",
				Endpoints:              []params.RemoteEndpoint{{Name: "db2"}}}},
		}})
	s.serviceBackend.CheckCallNames(c, listDirectoryOffersBackendCall)
}

var emptyFilterSet = params.OfferedApplicationFilters{
	Filters: []params.OfferedApplicationFilter{
		{FilterTerms: []params.OfferedApplicationFilterTerm{}},
	},
}

func (s *crossmodelSuite) TestList(c *gc.C) {
	serviceName := "test"
	url := "local:/u/fred/hosted-db2"

	s.addService(c, serviceName)

	s.serviceBackend.listOfferedApplications = func(filter ...crossmodel.OfferedApplicationFilter) ([]crossmodel.OfferedApplication, error) {
		return []crossmodel.OfferedApplication{
			{
				ApplicationName: serviceName,
				ApplicationURL:  url,
				Description:     "description",
				CharmName:       "charm",
				Endpoints:       map[string]string{"db": "db"},
			}}, nil
	}

	found, err := s.api.ListOffers(emptyFilterSet)
	c.Assert(err, jc.ErrorIsNil)
	s.serviceBackend.CheckCallNames(c, listOfferedApplicationsBackendCall)

	expectedService := params.OfferedApplicationDetails{
		CharmName:       "wordpress",
		UsersCount:      0,
		ApplicationName: serviceName,
		ApplicationURL:  url,
		Endpoints:       []params.RemoteEndpoint{{Name: "db", Role: "requirer", Interface: "mysql", Limit: 1, Scope: "global"}},
	}
	c.Assert(found, jc.DeepEquals,
		params.ListOffersResults{
			Results: []params.ListOffersFilterResults{{
				Result: []params.OfferedApplicationDetailsResult{
					{Result: &expectedService},
				}},
			},
		})
}

func (s *crossmodelSuite) TestListError(c *gc.C) {
	msg := "fail test"

	s.serviceBackend.listOfferedApplications = func(filter ...crossmodel.OfferedApplicationFilter) ([]crossmodel.OfferedApplication, error) {
		return nil, errors.New(msg)
	}

	found, err := s.api.ListOffers(emptyFilterSet)
	c.Assert(err, jc.ErrorIsNil)
	s.serviceBackend.CheckCallNames(c, listOfferedApplicationsBackendCall)
	c.Assert(found.Results[0].Error, gc.ErrorMatches, fmt.Sprintf("%v", msg))
}

func (s *crossmodelSuite) TestFind(c *gc.C) {
	serviceName := "test"
	url := "local:/u/fred/hosted-db2"

	filter := params.OfferFilterParams{
		Filters: []params.OfferFilters{
			{
				Directory: "local",
				Filters: []params.OfferFilter{
					{
						ApplicationURL:  "local:/u/fred/hosted-db2",
						ApplicationName: "test",
					},
				},
			},
		},
	}
	anOffer := params.ApplicationOffer{
		ApplicationName:        serviceName,
		ApplicationDescription: "description",
		ApplicationURL:         url,
		SourceModelTag:         "environment-",
		SourceLabel:            "label",
		Endpoints:              []params.RemoteEndpoint{{Name: "db"}},
	}

	s.serviceBackend.listDirectoryOffers = func(filter params.OfferFilters) (params.ApplicationOfferResults, error) {
		c.Assert(filter, jc.DeepEquals, params.OfferFilters{
			Directory: "local",
			Filters: []params.OfferFilter{
				{
					ApplicationURL:  "local:/u/fred/hosted-db2",
					ApplicationName: "test",
				},
			},
		})
		return params.ApplicationOfferResults{Offers: []params.ApplicationOffer{anOffer}}, nil
	}

	found, err := s.api.FindApplicationOffers(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, gc.DeepEquals,
		params.FindApplicationOffersResults{
			Results: []params.ApplicationOfferResults{
				{
					Offers: []params.ApplicationOffer{
						{
							ApplicationName:        serviceName,
							ApplicationDescription: "description",
							ApplicationURL:         url,
							SourceModelTag:         "environment-",
							SourceLabel:            "label",
							Endpoints:              []params.RemoteEndpoint{{Name: "db"}}}},
				}}})
	s.serviceBackend.CheckCallNames(c, listDirectoryOffersBackendCall)
}

func (s *crossmodelSuite) TestFindError(c *gc.C) {
	filter := params.OfferFilterParams{Filters: []params.OfferFilters{{}}}
	msg := "fail"

	s.serviceBackend.listDirectoryOffers = func(filter params.OfferFilters) (params.ApplicationOfferResults, error) {
		return params.ApplicationOfferResults{}, errors.New(msg)
	}

	found, err := s.api.FindApplicationOffers(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0].Error, gc.ErrorMatches, fmt.Sprintf(".*%v.*", msg))
	s.serviceBackend.CheckCallNames(c, listDirectoryOffersBackendCall)
}
