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
	"github.com/juju/juju/model/crossmodel"
)

type crossmodelSuite struct {
	baseCrossmodelSuite
}

var _ = gc.Suite(&crossmodelSuite{})

func (s *crossmodelSuite) SetUpTest(c *gc.C) {
	s.baseCrossmodelSuite.SetUpTest(c)
}

func (s *crossmodelSuite) TestOffer(c *gc.C) {
	serviceName := "test"
	expectedOffer := s.addService(c, serviceName)
	one := params.RemoteServiceOffer{
		ServiceURL:  "local:/u/me/test",
		ServiceName: serviceName,
		Endpoints:   []string{"db"},
	}
	all := params.RemoteServiceOffers{[]params.RemoteServiceOffer{one}}
	expectedOfferParams := params.AddServiceOffer{
		ServiceOffer: params.ServiceOffer{
			ServiceURL:         "local:/u/me/test",
			SourceEnvironTag:   "environment-deadbeef-0bad-400d-8000-4b1d0d06f00d",
			SourceLabel:        "dummyenv",
			ServiceName:        "test",
			ServiceDescription: "A pretty popular blog engine",
			Endpoints: []params.RemoteEndpoint{
				{Name: "db",
					Role:      "requirer",
					Interface: "mysql",
					Limit:     1,
					Scope:     "global",
				},
			}}}

	s.serviceBackend.addOffer = func(offer crossmodel.OfferedService, offerParams params.AddServiceOffer) error {
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
	one := params.RemoteServiceOffer{
		ServiceName: serviceName,
	}
	all := params.RemoteServiceOffers{[]params.RemoteServiceOffer{one}}

	msg := "fail"

	s.serviceBackend.addOffer = func(offer crossmodel.OfferedService, offerParams params.AddServiceOffer) error {
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

	filter := params.ShowFilter{[]string{url}}
	anOffer := params.ServiceOffer{
		ServiceName:        serviceName,
		ServiceDescription: "description",
		ServiceURL:         url,
		SourceEnvironTag:   "environment-",
		SourceLabel:        "label",
		Endpoints:          []params.RemoteEndpoint{{Name: "db"}},
	}

	s.serviceBackend.listDirectoryOffers = func(filter params.OfferFilters) (params.ServiceOfferResults, error) {
		return params.ServiceOfferResults{Offers: []params.ServiceOffer{anOffer}}, nil
	}

	found, err := s.api.Show(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, gc.DeepEquals,
		params.RemoteServiceResults{[]params.RemoteServiceResult{
			{Result: params.ServiceOffer{
				ServiceName:        serviceName,
				ServiceDescription: "description",
				ServiceURL:         url,
				SourceEnvironTag:   "environment-",
				SourceLabel:        "label",
				Endpoints:          []params.RemoteEndpoint{{Name: "db"}}}},
		}})
	s.serviceBackend.CheckCallNames(c, listDirectoryOffersBackendCall)
}

func (s *crossmodelSuite) TestShowError(c *gc.C) {
	url := "local:/u/fred/hosted-db2"
	filter := params.ShowFilter{[]string{url}}
	msg := "fail"

	s.serviceBackend.listDirectoryOffers = func(filter params.OfferFilters) (params.ServiceOfferResults, error) {
		return params.ServiceOfferResults{}, errors.New(msg)
	}

	found, err := s.api.Show(filter)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf(".*%v.*", msg))
	c.Assert(found.Results, gc.HasLen, 0)
	s.serviceBackend.CheckCallNames(c, listDirectoryOffersBackendCall)
}

func (s *crossmodelSuite) TestShowNotFound(c *gc.C) {
	urls := []string{"local:/u/fred/hosted-db2"}
	filter := params.ShowFilter{urls}

	s.serviceBackend.listDirectoryOffers = func(filter params.OfferFilters) (params.ServiceOfferResults, error) {
		return params.ServiceOfferResults{}, nil
	}

	found, err := s.api.Show(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0].Error.Error(), gc.Matches, fmt.Sprintf(`offer for remote service url %v not found`, urls[0]))
	s.serviceBackend.CheckCallNames(c, listDirectoryOffersBackendCall)
}

func (s *crossmodelSuite) TestShowErrorMsgMultipleURLs(c *gc.C) {
	urls := []string{"local:/u/fred/prod/foo/hosted-db2", "local:/u/fred/hosted-db2"}
	filter := params.ShowFilter{urls}

	s.serviceBackend.listDirectoryOffers = func(filter params.OfferFilters) (params.ServiceOfferResults, error) {
		return params.ServiceOfferResults{}, nil
	}

	found, err := s.api.Show(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 2)
	c.Assert(found.Results[0].Error.Error(), gc.Matches, fmt.Sprintf(`service URL has invalid form: %q`, urls[0]))
	c.Assert(found.Results[1].Error.Error(), gc.Matches, fmt.Sprintf(`offer for remote service url %v not found`, urls[1]))
	s.serviceBackend.CheckCallNames(c, listDirectoryOffersBackendCall)
}

func (s *crossmodelSuite) TestShowNotFoundEmpty(c *gc.C) {
	urls := []string{"local:/u/fred/hosted-db2"}
	filter := params.ShowFilter{urls}

	s.serviceBackend.listDirectoryOffers = func(filter params.OfferFilters) (params.ServiceOfferResults, error) {
		return params.ServiceOfferResults{}, nil
	}

	found, err := s.api.Show(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0].Error.Error(), gc.Matches, fmt.Sprintf(`offer for remote service url %v not found`, urls[0]))
	s.serviceBackend.CheckCallNames(c, listDirectoryOffersBackendCall)
}

func (s *crossmodelSuite) TestShowFoundMultiple(c *gc.C) {
	name := "test"
	url := "local:/u/fred/hosted-db2"
	anOffer := params.ServiceOffer{
		ServiceName:        name,
		ServiceDescription: "description",
		ServiceURL:         url,
		SourceEnvironTag:   "environment-",
		SourceLabel:        "label",
		Endpoints:          []params.RemoteEndpoint{{Name: "db"}},
	}

	name2 := "testAgain"
	url2 := "local:/u/mary/hosted-db2"
	anOffer2 := params.ServiceOffer{
		ServiceName:        name2,
		ServiceDescription: "description2",
		ServiceURL:         url2,
		SourceEnvironTag:   "environment-",
		SourceLabel:        "label2",
		Endpoints:          []params.RemoteEndpoint{{Name: "db2"}},
	}

	filter := params.ShowFilter{[]string{url, url2}}

	s.serviceBackend.listDirectoryOffers = func(filter params.OfferFilters) (params.ServiceOfferResults, error) {
		return params.ServiceOfferResults{Offers: []params.ServiceOffer{anOffer, anOffer2}}, nil
	}

	found, err := s.api.Show(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, gc.DeepEquals, params.RemoteServiceResults{
		[]params.RemoteServiceResult{
			{Result: params.ServiceOffer{
				ServiceName:        name,
				ServiceDescription: "description",
				ServiceURL:         url,
				SourceEnvironTag:   "environment-",
				SourceLabel:        "label",
				Endpoints:          []params.RemoteEndpoint{{Name: "db"}}}},
			{Result: params.ServiceOffer{
				ServiceName:        name2,
				ServiceDescription: "description2",
				ServiceURL:         url2,
				SourceEnvironTag:   "environment-",
				SourceLabel:        "label2",
				Endpoints:          []params.RemoteEndpoint{{Name: "db2"}}}},
		}})
	s.serviceBackend.CheckCallNames(c, listDirectoryOffersBackendCall)
}

var emptyFilterSet = params.ListEndpointsFilters{
	Filters: []params.ListEndpointsFilter{
		{FilterTerms: []params.ListEndpointsFilterTerm{}},
	},
}

func (s *crossmodelSuite) TestList(c *gc.C) {
	serviceName := "test"
	url := "local:/u/fred/hosted-db2"

	s.addService(c, serviceName)

	s.serviceBackend.listOfferedServices = func(filter ...crossmodel.OfferedServiceFilter) ([]crossmodel.OfferedService, error) {
		return []crossmodel.OfferedService{
			{
				ServiceName: serviceName,
				ServiceURL:  url,
				Description: "description",
				CharmName:   "charm",
				Endpoints:   map[string]string{"db": "db"},
			}}, nil
	}

	found, err := s.api.List(emptyFilterSet)
	c.Assert(err, jc.ErrorIsNil)
	s.serviceBackend.CheckCallNames(c, listOfferedServicesBackendCall)

	expectedService := params.ListEndpointsServiceItem{
		CharmName:   "wordpress",
		UsersCount:  0,
		ServiceName: serviceName,
		ServiceURL:  url,
		Endpoints:   []charm.Relation{{Name: "db", Role: "requirer", Interface: "mysql", Limit: 1, Scope: "global"}},
	}
	c.Assert(found, jc.DeepEquals,
		params.ListEndpointsItemsResults{
			Results: []params.ListEndpointsItemsResult{{
				Result: []params.ListEndpointsServiceItemResult{
					{Result: &expectedService},
				}},
			},
		})
}

func (s *crossmodelSuite) TestListError(c *gc.C) {
	msg := "fail test"

	s.serviceBackend.listOfferedServices = func(filter ...crossmodel.OfferedServiceFilter) ([]crossmodel.OfferedService, error) {
		return nil, errors.New(msg)
	}

	found, err := s.api.List(emptyFilterSet)
	c.Assert(err, jc.ErrorIsNil)
	s.serviceBackend.CheckCallNames(c, listOfferedServicesBackendCall)
	c.Assert(found.Results[0].Error, gc.ErrorMatches, fmt.Sprintf("%v", msg))
}
