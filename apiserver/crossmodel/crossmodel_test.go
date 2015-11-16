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
	expectedOffer := s.stateAccess.addService(c, s.JujuConnSuite, serviceName)
	one := params.RemoteServiceOffer{
		ServiceName: serviceName,
	}
	all := params.RemoteServiceOffers{[]params.RemoteServiceOffer{one}}

	s.serviceDirectory.addOffer = func(offer crossmodel.ServiceOffer) error {
		//		stored = offer
		c.Assert(offer, gc.DeepEquals, expectedOffer)
		return nil
	}

	errs, err := s.api.Offer(all)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs.Results, gc.HasLen, len(all.Offers))
	c.Assert(errs.Results[0].Error, gc.IsNil)
	s.serviceDirectory.CheckCallNames(c, addOfferCall)
	s.stateAccess.CheckCallNames(c, environConfigCall, serviceCall, environUUIDCall)
}

func (s *crossmodelSuite) TestOfferError(c *gc.C) {
	serviceName := "test"
	s.stateAccess.addService(c, s.JujuConnSuite, serviceName)
	one := params.RemoteServiceOffer{
		ServiceName: serviceName,
	}
	all := params.RemoteServiceOffers{[]params.RemoteServiceOffer{one}}

	msg := "fail"

	s.serviceDirectory.addOffer = func(offer crossmodel.ServiceOffer) error {
		return errors.New(msg)
	}

	errs, err := s.api.Offer(all)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs.Results, gc.HasLen, len(all.Offers))
	c.Assert(errs.Results[0].Error, gc.ErrorMatches, fmt.Sprintf(".*%v.*", msg))
	s.serviceDirectory.CheckCallNames(c, addOfferCall)
	s.stateAccess.CheckCallNames(c, environConfigCall, serviceCall, environUUIDCall)
}

func (s *crossmodelSuite) TestShow(c *gc.C) {
	serviceName := "test"
	url := "local:/u/fred/hosted-db2"
	anOffer := crossmodel.ServiceOffer{ServiceName: serviceName, ServiceURL: url}

	s.serviceDirectory.listOffers = func(filter ...crossmodel.ServiceOfferFilter) ([]crossmodel.ServiceOffer, error) {
		return []crossmodel.ServiceOffer{anOffer}, nil
	}

	found, err := s.api.Show([]string{url})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, gc.DeepEquals,
		params.RemoteServiceResults{[]params.RemoteServiceResult{
			{Result: params.ServiceOffer{
				ServiceName:      serviceName,
				ServiceURL:       url,
				SourceEnvironTag: "environment-",
				Endpoints:        []params.RemoteEndpoint{}}},
		}})
	s.serviceDirectory.CheckCallNames(c, listOffersCall)
}

func (s *crossmodelSuite) TestShowError(c *gc.C) {
	url := "local:/u/fred/hosted-db2"
	msg := "fail"

	s.serviceDirectory.listOffers = func(filter ...crossmodel.ServiceOfferFilter) ([]crossmodel.ServiceOffer, error) {
		return nil, errors.New(msg)
	}

	found, err := s.api.Show([]string{url})
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf(".*%v.*", msg))
	c.Assert(found.Results, gc.HasLen, 0)
	s.serviceDirectory.CheckCallNames(c, listOffersCall)
}

func (s *crossmodelSuite) TestShowNotFound(c *gc.C) {
	urls := []string{"local:/u/fred/hosted-db2"}

	s.serviceDirectory.listOffers = func(filter ...crossmodel.ServiceOfferFilter) ([]crossmodel.ServiceOffer, error) {
		return nil, nil
	}

	found, err := s.api.Show(urls)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0].Error.Error(), gc.Matches, fmt.Sprintf(`offer for remote service url %v not found`, urls[0]))
	s.serviceDirectory.CheckCallNames(c, listOffersCall)
}

func (s *crossmodelSuite) TestShowErrorMsgMultipleURLs(c *gc.C) {
	urls := []string{"local:/u/fred/prod/hosted-db2", "local:/u/fred/hosted-db2"}

	s.serviceDirectory.listOffers = func(filter ...crossmodel.ServiceOfferFilter) ([]crossmodel.ServiceOffer, error) {
		return nil, nil
	}

	found, err := s.api.Show(urls)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 2)
	c.Assert(found.Results[0].Error.Error(), gc.Matches, fmt.Sprintf(`service URL has invalid form: %q`, urls[0]))
	c.Assert(found.Results[1].Error.Error(), gc.Matches, fmt.Sprintf(`offer for remote service url %v not found`, urls[1]))
	s.serviceDirectory.CheckCallNames(c, listOffersCall)
}

func (s *crossmodelSuite) TestShowNotFoundEmpty(c *gc.C) {
	urls := []string{"local:/u/fred/hosted-db2"}

	s.serviceDirectory.listOffers = func(filter ...crossmodel.ServiceOfferFilter) ([]crossmodel.ServiceOffer, error) {
		return []crossmodel.ServiceOffer{}, nil
	}

	found, err := s.api.Show(urls)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0].Error.Error(), gc.Matches, fmt.Sprintf(`offer for remote service url %v not found`, urls[0]))
	s.serviceDirectory.CheckCallNames(c, listOffersCall)
}

func (s *crossmodelSuite) TestShowFoundMultiple(c *gc.C) {
	name := "test"
	url := "local:/u/fred/hosted-db2"
	anOffer := crossmodel.ServiceOffer{ServiceName: name, ServiceURL: url}

	name2 := "testAgain"
	url2 := "local:/u/mary/hosted-db2"
	anOffer2 := crossmodel.ServiceOffer{ServiceName: name2, ServiceURL: url2}

	s.serviceDirectory.listOffers = func(filter ...crossmodel.ServiceOfferFilter) ([]crossmodel.ServiceOffer, error) {
		return []crossmodel.ServiceOffer{anOffer, anOffer2}, nil
	}

	found, err := s.api.Show([]string{url, url2})
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
	s.serviceDirectory.CheckCallNames(c, listOffersCall)
}
