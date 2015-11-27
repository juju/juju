// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"fmt"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

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
		ServiceName: serviceName,
	}
	all := params.RemoteServiceOffers{[]params.RemoteServiceOffer{one}}

	s.serviceBackend.addOffer = func(offer crossmodel.ServiceOffer) error {
		c.Assert(offer, gc.DeepEquals, expectedOffer)
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

	s.serviceBackend.addOffer = func(offer crossmodel.ServiceOffer) error {
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
	anOffer := crossmodel.ServiceOffer{ServiceName: serviceName, ServiceURL: url}

	s.serviceBackend.listOffers = func(f ...crossmodel.ServiceOfferFilter) ([]crossmodel.ServiceOffer, error) {
		return []crossmodel.ServiceOffer{anOffer}, nil
	}

	found, err := s.api.Show(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, gc.DeepEquals,
		params.RemoteServiceResults{[]params.RemoteServiceResult{
			{Result: params.ServiceOffer{
				ServiceName:      serviceName,
				ServiceURL:       url,
				SourceEnvironTag: "environment-",
				Endpoints:        []params.RemoteEndpoint{}}},
		}})
	s.serviceBackend.CheckCallNames(c, listOffersBackendCall)
}

func (s *crossmodelSuite) TestShowError(c *gc.C) {
	url := "local:/u/fred/hosted-db2"
	filter := params.ShowFilter{[]string{url}}
	msg := "fail"

	s.serviceBackend.listOffers = func(f ...crossmodel.ServiceOfferFilter) ([]crossmodel.ServiceOffer, error) {
		return nil, errors.New(msg)
	}

	found, err := s.api.Show(filter)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf(".*%v.*", msg))
	c.Assert(found.Results, gc.HasLen, 0)
	s.serviceBackend.CheckCallNames(c, listOffersBackendCall)
}

func (s *crossmodelSuite) TestShowNotFound(c *gc.C) {
	urls := []string{"local:/u/fred/hosted-db2"}
	filter := params.ShowFilter{urls}

	s.serviceBackend.listOffers = func(filter ...crossmodel.ServiceOfferFilter) ([]crossmodel.ServiceOffer, error) {
		return nil, nil
	}

	found, err := s.api.Show(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0].Error.Error(), gc.Matches, fmt.Sprintf(`offer for remote service url %v not found`, urls[0]))
	s.serviceBackend.CheckCallNames(c, listOffersBackendCall)
}

func (s *crossmodelSuite) TestShowErrorMsgMultipleURLs(c *gc.C) {
	urls := []string{"local:/u/fred/prod/hosted-db2", "local:/u/fred/hosted-db2"}
	filter := params.ShowFilter{urls}

	s.serviceBackend.listOffers = func(f ...crossmodel.ServiceOfferFilter) ([]crossmodel.ServiceOffer, error) {
		return nil, nil
	}

	found, err := s.api.Show(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 2)
	c.Assert(found.Results[0].Error.Error(), gc.Matches, fmt.Sprintf(`service URL has invalid form: %q`, urls[0]))
	c.Assert(found.Results[1].Error.Error(), gc.Matches, fmt.Sprintf(`offer for remote service url %v not found`, urls[1]))
	s.serviceBackend.CheckCallNames(c, listOffersBackendCall)
}

func (s *crossmodelSuite) TestShowNotFoundEmpty(c *gc.C) {
	urls := []string{"local:/u/fred/hosted-db2"}
	filter := params.ShowFilter{urls}

	s.serviceBackend.listOffers = func(f ...crossmodel.ServiceOfferFilter) ([]crossmodel.ServiceOffer, error) {
		return []crossmodel.ServiceOffer{}, nil
	}

	found, err := s.api.Show(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0].Error.Error(), gc.Matches, fmt.Sprintf(`offer for remote service url %v not found`, urls[0]))
	s.serviceBackend.CheckCallNames(c, listOffersBackendCall)
}

func (s *crossmodelSuite) TestShowFoundMultiple(c *gc.C) {
	name := "test"
	url := "local:/u/fred/hosted-db2"
	anOffer := crossmodel.ServiceOffer{ServiceName: name, ServiceURL: url}

	name2 := "testAgain"
	url2 := "local:/u/mary/hosted-db2"
	anOffer2 := crossmodel.ServiceOffer{ServiceName: name2, ServiceURL: url2}

	filter := params.ShowFilter{[]string{url, url2}}

	s.serviceBackend.listOffers = func(f ...crossmodel.ServiceOfferFilter) ([]crossmodel.ServiceOffer, error) {
		return []crossmodel.ServiceOffer{anOffer, anOffer2}, nil
	}

	found, err := s.api.Show(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, gc.DeepEquals, params.RemoteServiceResults{
		[]params.RemoteServiceResult{
			{Result: params.ServiceOffer{
				ServiceName:      name,
				ServiceURL:       url,
				SourceEnvironTag: "environment-",
				Endpoints:        []params.RemoteEndpoint{}}},
			{Result: params.ServiceOffer{
				ServiceName:      name2,
				ServiceURL:       url2,
				SourceEnvironTag: "environment-",
				Endpoints:        []params.RemoteEndpoint{}}},
		}})
	s.serviceBackend.CheckCallNames(c, listOffersBackendCall)
}

var emptyFilterSet = params.ListEndpointsFilters{
	Filters: []params.ListEndpointsFilter{
		{FilterTerms: []params.ListEndpointsFilterTerm{}},
	},
}

func (s *crossmodelSuite) TestList(c *gc.C) {
	serviceName := "test"
	url := "local:/u/fred/hosted-db2"
	connectedUsers := []string{"user1", "user2"}

	s.addService(c, serviceName)

	remote := crossmodel.RemoteService{crossmodel.ServiceOffer{ServiceName: serviceName, ServiceURL: url}, connectedUsers}

	s.serviceBackend.listRemoteServices = func(filters ...crossmodel.RemoteServiceFilter) ([]crossmodel.RemoteService, error) {
		return []crossmodel.RemoteService{remote}, nil
	}

	found, err := s.api.List(emptyFilterSet)
	c.Assert(err, jc.ErrorIsNil)
	s.serviceBackend.CheckCallNames(c, listRemoteServicesBackendCall)

	expectedService := params.ListEndpointsServiceItem{
		CharmName:   "wordpress",
		UsersCount:  len(connectedUsers),
		ServiceName: serviceName,
		ServiceURL:  url,
	}
	c.Assert(found, gc.DeepEquals,
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

	s.serviceBackend.listRemoteServices = func(filters ...crossmodel.RemoteServiceFilter) ([]crossmodel.RemoteService, error) {
		return nil, errors.New(msg)
	}

	found, err := s.api.List(emptyFilterSet)
	c.Assert(err, jc.ErrorIsNil)
	s.serviceBackend.CheckCallNames(c, listRemoteServicesBackendCall)
	c.Assert(found.Results[0].Error, gc.ErrorMatches, fmt.Sprintf("%v", msg))
}
