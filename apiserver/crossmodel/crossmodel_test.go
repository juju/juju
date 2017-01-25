// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"fmt"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/crossmodel"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/testing"
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
	one := params.ApplicationOfferParams{
		ModelTag:        "model-deadbeef-0bad-400d-8000-4b1d0d06f00d",
		ApplicationURL:  "local:/u/me/test",
		ApplicationName: applicationName,
		Endpoints:       []string{"db"},
	}
	all := params.ApplicationOffersParams{Offers: []params.ApplicationOfferParams{one}}
	expectedOffers := params.AddApplicationOffers{
		Offers: []params.AddApplicationOffer{{
			ApplicationOffer: params.ApplicationOffer{
				ApplicationURL:         "local:/u/me/test",
				SourceModelTag:         testing.ModelTag.String(),
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
				}}}}}
	s.applicationDirectory.addOffers = func(offers params.AddApplicationOffers) (params.ErrorResults, error) {
		c.Assert(offers, jc.DeepEquals, expectedOffers)
		result := params.ErrorResults{}
		result.Results = make([]params.ErrorResult, len(offers.Offers))
		return result, nil
	}

	errs, err := s.api.Offer(all)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs.Results, gc.HasLen, len(all.Offers))
	c.Assert(errs.Results[0].Error, gc.IsNil)
	s.applicationDirectory.CheckCallNames(c, addOffersBackendCall)
}

func (s *crossmodelSuite) TestOfferSomeFail(c *gc.C) {
	s.addApplication(c, "one")
	s.addApplication(c, "two")
	one := params.ApplicationOfferParams{
		ModelTag:        "model-deadbeef-0bad-400d-8000-4b1d0d06f00d",
		ApplicationURL:  "local:/u/me/one",
		ApplicationName: "one",
		Endpoints:       []string{"db"},
	}
	bad := params.ApplicationOfferParams{
		ModelTag:        "model-deadbeef-0bad-400d-8000-4b1d0d06f00d",
		ApplicationURL:  "local:/u/me/bad",
		ApplicationName: "notthere",
		Endpoints:       []string{"db"},
	}
	bad2 := params.ApplicationOfferParams{
		ModelTag:        "model-deadbeef-0bad-400d-8000-4b1d0d06f00d",
		ApplicationURL:  "local:/u/me/bad",
		ApplicationName: "paramsfail",
		Endpoints:       []string{"db"},
	}
	two := params.ApplicationOfferParams{
		ModelTag:        "model-deadbeef-0bad-400d-8000-4b1d0d06f00d",
		ApplicationURL:  "local:/u/me/two",
		ApplicationName: "two",
		Endpoints:       []string{"db"},
	}
	all := params.ApplicationOffersParams{Offers: []params.ApplicationOfferParams{one, bad, bad2, two}}
	expectedOffers := params.AddApplicationOffers{
		Offers: []params.AddApplicationOffer{{
			ApplicationOffer: params.ApplicationOffer{
				ApplicationURL:         "local:/u/me/one",
				SourceModelTag:         testing.ModelTag.String(),
				SourceLabel:            "controller",
				ApplicationName:        "one",
				ApplicationDescription: "A pretty popular blog engine",
				Endpoints: []params.RemoteEndpoint{
					{Name: "db",
						Role:      "requirer",
						Interface: "mysql",
						Limit:     1,
						Scope:     "global",
					},
				}}}, {
			ApplicationOffer: params.ApplicationOffer{
				ApplicationURL:         "local:/u/me/two",
				SourceModelTag:         testing.ModelTag.String(),
				SourceLabel:            "controller",
				ApplicationName:        "two",
				ApplicationDescription: "A pretty popular blog engine",
				Endpoints: []params.RemoteEndpoint{
					{Name: "db",
						Role:      "requirer",
						Interface: "mysql",
						Limit:     1,
						Scope:     "global",
					},
				}}}}}
	s.applicationDirectory.addOffers = func(offers params.AddApplicationOffers) (params.ErrorResults, error) {
		c.Assert(offers, jc.DeepEquals, expectedOffers)
		result := params.ErrorResults{}
		result.Results = make([]params.ErrorResult, len(offers.Offers))
		return result, nil
	}

	s.makeOfferedApplicationParamsFunc = func(p params.ApplicationOfferParams) (params.ApplicationOffer, error) {
		if p.ApplicationName == "paramsfail" {
			return params.ApplicationOffer{}, errors.New("params fail")
		}
		return crossmodel.MakeOfferedApplicationParams(s.api, p)
	}
	errs, err := s.api.Offer(all)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs.Results, gc.HasLen, len(all.Offers))
	c.Assert(errs.Results[0].Error, gc.IsNil)
	c.Assert(errs.Results[3].Error, gc.IsNil)
	c.Assert(errs.Results[1].Error, gc.ErrorMatches, `getting offered application notthere: application "notthere" not found`)
	c.Assert(errs.Results[2].Error, gc.ErrorMatches, `params fail`)
	s.applicationDirectory.CheckCallNames(c, addOffersBackendCall)
}

func (s *crossmodelSuite) TestOfferError(c *gc.C) {
	applicationName := "test"
	s.addApplication(c, applicationName)
	one := params.ApplicationOfferParams{
		ModelTag:        "model-deadbeef-0bad-400d-8000-4b1d0d06f00d",
		ApplicationName: applicationName,
	}
	all := params.ApplicationOffersParams{[]params.ApplicationOfferParams{one}}

	msg := "fail"

	s.applicationDirectory.addOffers = func(offers params.AddApplicationOffers) (params.ErrorResults, error) {
		return params.ErrorResults{Results: []params.ErrorResult{{Error: &params.Error{Message: msg}}}}, nil
	}

	errs, err := s.api.Offer(all)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs.Results, gc.HasLen, len(all.Offers))
	c.Assert(errs.Results[0].Error, gc.ErrorMatches, fmt.Sprintf(".*%v.*", msg))
	s.applicationDirectory.CheckCallNames(c, addOffersBackendCall)
}

func (s *crossmodelSuite) TestShow(c *gc.C) {
	applicationName := "test"
	url := "local:/u/fred/hosted-db2"

	filter := params.ApplicationURLs{[]string{url}}
	anOffer := params.ApplicationOffer{
		ApplicationName:        applicationName,
		ApplicationDescription: "description",
		ApplicationURL:         url,
		SourceModelTag:         "model-",
		SourceLabel:            "label",
		Endpoints:              []params.RemoteEndpoint{{Name: "db"}},
	}

	s.applicationDirectory.listOffers = func(filter params.OfferFilters) (params.ApplicationOfferResults, error) {
		return params.ApplicationOfferResults{Offers: []params.ApplicationOffer{anOffer}}, nil
	}

	found, err := s.api.ApplicationOffers(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, jc.DeepEquals,
		params.ApplicationOffersResults{[]params.ApplicationOfferResult{
			{Result: params.ApplicationOffer{
				ApplicationName:        applicationName,
				ApplicationDescription: "description",
				ApplicationURL:         url,
				SourceModelTag:         "model-",
				SourceLabel:            "label",
				Endpoints:              []params.RemoteEndpoint{{Name: "db"}}}},
		}})
	s.applicationDirectory.CheckCallNames(c, listOffersBackendCall)
}

func (s *crossmodelSuite) TestShowError(c *gc.C) {
	url := "local:/u/fred/hosted-db2"
	filter := params.ApplicationURLs{[]string{url}}
	msg := "fail"

	s.applicationDirectory.listOffers = func(filter params.OfferFilters) (params.ApplicationOfferResults, error) {
		return params.ApplicationOfferResults{}, errors.New(msg)
	}

	found, err := s.api.ApplicationOffers(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0].Error, gc.ErrorMatches, fmt.Sprintf(".*%v.*", msg))
	s.applicationDirectory.CheckCallNames(c, listOffersBackendCall)
}

func (s *crossmodelSuite) TestShowNotFound(c *gc.C) {
	urls := []string{"local:/u/fred/hosted-db2"}
	filter := params.ApplicationURLs{urls}

	s.applicationDirectory.listOffers = func(filter params.OfferFilters) (params.ApplicationOfferResults, error) {
		return params.ApplicationOfferResults{}, nil
	}

	found, err := s.api.ApplicationOffers(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0].Error.Error(), gc.Matches, fmt.Sprintf(`offer for remote application url %v not found`, urls[0]))
	s.applicationDirectory.CheckCallNames(c, listOffersBackendCall)
}

func (s *crossmodelSuite) TestShowErrorMsgMultipleURLs(c *gc.C) {
	urls := []string{"local:/u/fred/prod/hosted-db2", "local:/u/fred/hosted-db2"}
	filter := params.ApplicationURLs{urls}

	s.applicationDirectory.listOffers = func(filter params.OfferFilters) (params.ApplicationOfferResults, error) {
		return params.ApplicationOfferResults{}, nil
	}

	found, err := s.api.ApplicationOffers(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 2)
	c.Assert(found.Results[0].Error.Error(), gc.Matches, fmt.Sprintf(`application URL has too many parts: %q`, urls[0]))
	c.Assert(found.Results[1].Error.Error(), gc.Matches, fmt.Sprintf(`offer for remote application url %v not found`, urls[1]))
	s.applicationDirectory.CheckCallNames(c, listOffersBackendCall)
}

func (s *crossmodelSuite) TestShowNotFoundEmpty(c *gc.C) {
	urls := []string{"local:/u/fred/hosted-db2"}
	filter := params.ApplicationURLs{urls}

	s.applicationDirectory.listOffers = func(filter params.OfferFilters) (params.ApplicationOfferResults, error) {
		return params.ApplicationOfferResults{}, nil
	}

	found, err := s.api.ApplicationOffers(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0].Error.Error(), gc.Matches, fmt.Sprintf(`offer for remote application url %v not found`, urls[0]))
	s.applicationDirectory.CheckCallNames(c, listOffersBackendCall)
}

func (s *crossmodelSuite) TestShowFoundMultiple(c *gc.C) {
	name := "test"
	url := "local:/u/fred/hosted-db2"
	anOffer := params.ApplicationOffer{
		ApplicationName:        name,
		ApplicationDescription: "description",
		ApplicationURL:         url,
		SourceModelTag:         "model-",
		SourceLabel:            "label",
		Endpoints:              []params.RemoteEndpoint{{Name: "db"}},
	}

	name2 := "testAgain"
	url2 := "local:/u/mary/hosted-db2"
	anOffer2 := params.ApplicationOffer{
		ApplicationName:        name2,
		ApplicationDescription: "description2",
		ApplicationURL:         url2,
		SourceModelTag:         "model-",
		SourceLabel:            "label2",
		Endpoints:              []params.RemoteEndpoint{{Name: "db2"}},
	}

	filter := params.ApplicationURLs{[]string{url, url2}}

	s.applicationDirectory.listOffers = func(filter params.OfferFilters) (params.ApplicationOfferResults, error) {
		return params.ApplicationOfferResults{Offers: []params.ApplicationOffer{anOffer, anOffer2}}, nil
	}

	found, err := s.api.ApplicationOffers(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, jc.DeepEquals, params.ApplicationOffersResults{
		[]params.ApplicationOfferResult{
			{Result: params.ApplicationOffer{
				ApplicationName:        name,
				ApplicationDescription: "description",
				ApplicationURL:         url,
				SourceModelTag:         "model-",
				SourceLabel:            "label",
				Endpoints:              []params.RemoteEndpoint{{Name: "db"}}}},
			{Result: params.ApplicationOffer{
				ApplicationName:        name2,
				ApplicationDescription: "description2",
				ApplicationURL:         url2,
				SourceModelTag:         "model-",
				SourceLabel:            "label2",
				Endpoints:              []params.RemoteEndpoint{{Name: "db2"}}}},
		}})
	s.applicationDirectory.CheckCallNames(c, listOffersBackendCall)
}

var emptyFilterSet1 = params.OfferedApplicationFilters{
	Filters: []params.OfferedApplicationFilter{
		{FilterTerms: []params.OfferedApplicationFilterTerm{}},
	},
}

func (s *crossmodelSuite) TestFind(c *gc.C) {
	applicationName := "test"
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
		ApplicationName:        applicationName,
		ApplicationDescription: "description",
		ApplicationURL:         url,
		SourceModelTag:         "model-",
		SourceLabel:            "label",
		Endpoints:              []params.RemoteEndpoint{{Name: "db"}},
	}

	s.applicationDirectory.listOffers = func(filter params.OfferFilters) (params.ApplicationOfferResults, error) {
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
	c.Assert(found, jc.DeepEquals,
		params.FindApplicationOffersResults{
			Results: []params.ApplicationOfferResults{
				{
					Offers: []params.ApplicationOffer{
						{
							ApplicationName:        applicationName,
							ApplicationDescription: "description",
							ApplicationURL:         url,
							SourceModelTag:         "model-",
							SourceLabel:            "label",
							Endpoints:              []params.RemoteEndpoint{{Name: "db"}}}},
				}}})
	s.applicationDirectory.CheckCallNames(c, listOffersBackendCall)
}

func (s *crossmodelSuite) TestFindError(c *gc.C) {
	filter := params.OfferFilterParams{Filters: []params.OfferFilters{{}}}
	msg := "fail"

	s.applicationDirectory.listOffers = func(filter params.OfferFilters) (params.ApplicationOfferResults, error) {
		return params.ApplicationOfferResults{}, errors.New(msg)
	}

	found, err := s.api.FindApplicationOffers(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0].Error, gc.ErrorMatches, fmt.Sprintf(".*%v.*", msg))
	s.applicationDirectory.CheckCallNames(c, listOffersBackendCall)
}
