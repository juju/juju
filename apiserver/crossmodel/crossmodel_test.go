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

	crossmodel.TempPlaceholder = make(map[names.ServiceTag]crossmodel.Offer)
}

func (s *crossmodelSuite) TestOffer(c *gc.C) {
	serviceName := "test"
	expectedOffer := s.stateAccess.addService(c, s.JujuConnSuite, serviceName)
	one := params.CrossModelOffer{serviceName, nil, "", nil}
	all := params.CrossModelOffers{[]params.CrossModelOffer{one}}

	//	var stored crossmodel.ServiceOffer
	s.serviceDirectory.addOffer = func(offer crossmodel.ServiceOffer) error {
		//		stored = offer
		c.Assert(offer, gc.DeepEquals, expectedOffer)
		return nil
	}

	errs, err := s.api.Offer(all)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs.Results, gc.HasLen, len(all.Offers))
	c.Assert(errs.Results[0].Error, gc.IsNil)
	//	c.Assert(stored, gc.DeepEquals, expectedOffer)
	s.serviceDirectory.CheckCallNames(c, addOfferCall)
	s.stateAccess.CheckCallNames(c, environConfigCall, serviceCall, environUUIDCall)
}

func (s *crossmodelSuite) TestOfferError(c *gc.C) {
	serviceName := "test"
	s.stateAccess.addService(c, s.JujuConnSuite, serviceName)
	one := params.CrossModelOffer{serviceName, nil, "", nil}
	all := params.CrossModelOffers{[]params.CrossModelOffer{one}}

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

func (s *crossmodelSuite) TestListOffers(c *gc.C) {
	serviceName := "test"
	url := "local:/u/fred/prod/hosted-db2"
	anOffer := crossmodel.ServiceOffer{ServiceName: serviceName, ServiceURL: url}

	s.serviceDirectory.listOffers = func(filter ...crossmodel.ServiceOfferFilter) ([]crossmodel.ServiceOffer, error) {
		return []crossmodel.ServiceOffer{anOffer}, nil
	}

	found, err := s.api.Show([]string{url})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, gc.DeepEquals,
		params.RemoteServiceInfoResults{[]params.RemoteServiceInfoResult{
			{Result: params.RemoteServiceInfo{Service: serviceName, Endpoints: []params.RemoteEndpoint{}}},
		}})
	s.serviceDirectory.CheckCallNames(c, listOffersCall)
}

func (s *crossmodelSuite) TestListOffersError(c *gc.C) {
	url := "local:/u/fred/prod/hosted-db2"
	msg := "fail"

	s.serviceDirectory.listOffers = func(filter ...crossmodel.ServiceOfferFilter) ([]crossmodel.ServiceOffer, error) {
		return nil, errors.New(msg)
	}

	found, err := s.api.Show([]string{url})
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf(".*%v.*", msg))
	c.Assert(found.Results, gc.HasLen, 0)
	s.serviceDirectory.CheckCallNames(c, listOffersCall)
}

func (s *crossmodelSuite) TestListOffersNotFound(c *gc.C) {
	urls := []string{"local:/u/fred/prod/hosted-db2"}

	s.serviceDirectory.listOffers = func(filter ...crossmodel.ServiceOfferFilter) ([]crossmodel.ServiceOffer, error) {
		return nil, nil
	}

	found, err := s.api.Show(urls)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0].Error.Error(), gc.Matches, fmt.Sprintf(`offer for remote service url %v not found`, urls[0]))
	s.serviceDirectory.CheckCallNames(c, listOffersCall)
}

func (s *crossmodelSuite) TestListOffersErrorMsgMultipleURLs(c *gc.C) {
	urls := []string{"local:/u/fred/prod/hosted-db2", "remote:/u/fred/hosted-db2"}

	s.serviceDirectory.listOffers = func(filter ...crossmodel.ServiceOfferFilter) ([]crossmodel.ServiceOffer, error) {
		return nil, nil
	}

	found, err := s.api.Show(urls)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 2)
	c.Assert(found.Results[0].Error.Error(), gc.Matches, fmt.Sprintf(`offer for remote service url %v not found`, urls[0]))
	c.Assert(found.Results[1].Error.Error(), gc.Matches, fmt.Sprintf(`offer for remote service url %v not found`, urls[1]))
	s.serviceDirectory.CheckCallNames(c, listOffersCall)
}

func (s *crossmodelSuite) TestListOffersNotFoundEmpty(c *gc.C) {
	urls := []string{"local:/u/fred/prod/hosted-db2"}

	s.serviceDirectory.listOffers = func(filter ...crossmodel.ServiceOfferFilter) ([]crossmodel.ServiceOffer, error) {
		return []crossmodel.ServiceOffer{}, nil
	}

	found, err := s.api.Show(urls)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.Results, gc.HasLen, 1)
	c.Assert(found.Results[0].Error.Error(), gc.Matches, fmt.Sprintf(`offer for remote service url %v not found`, urls[0]))
	s.serviceDirectory.CheckCallNames(c, listOffersCall)
}

func (s *crossmodelSuite) TestListOffersFoundMultiple(c *gc.C) {
	name := "test"
	url := "local:/u/fred/prod/hosted-db2"
	anOffer := crossmodel.ServiceOffer{ServiceName: name, ServiceURL: url}

	name2 := "testAgain"
	url2 := "local:/u/mary/prod/hosted-db2"
	anOffer2 := crossmodel.ServiceOffer{ServiceName: name2, ServiceURL: url2}

	s.serviceDirectory.listOffers = func(filter ...crossmodel.ServiceOfferFilter) ([]crossmodel.ServiceOffer, error) {
		return []crossmodel.ServiceOffer{anOffer, anOffer2}, nil
	}

	found, err := s.api.Show([]string{url, url2})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, gc.DeepEquals, params.RemoteServiceInfoResults{
		[]params.RemoteServiceInfoResult{
			{Result: params.RemoteServiceInfo{Service: name, Endpoints: []params.RemoteEndpoint{}}},
			{Result: params.RemoteServiceInfo{Service: name2, Endpoints: []params.RemoteEndpoint{}}},
		}})
	s.serviceDirectory.CheckCallNames(c, listOffersCall)
}

func (s *crossmodelSuite) TestShow(c *gc.C) {
	tag := names.NewServiceTag("test")
	url := "local:/u/fred/prod/hosted-db2"
	anOffer := crossmodel.NewOffer(tag, nil, url, nil)

	crossmodel.TempPlaceholder[tag] = anOffer

	found, err := s.api.Show(params.EndpointsSearchFilter{URL: url})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, gc.DeepEquals, params.EndpointsDetailsResult{Service: tag.String()})
}

func (s *crossmodelSuite) TestShowNotFound(c *gc.C) {
	url := "local:/u/fred/prod/hosted-db2"

	found, err := s.api.Show(params.EndpointsSearchFilter{URL: url})
	c.Assert(err, gc.ErrorMatches, `endpoints with url "local:/u/fred/prod/hosted-db2" not found`)
	c.Assert(found, gc.DeepEquals, params.EndpointsDetailsResult{})
}

func (s *crossmodelSuite) TestShowFoundMultiple(c *gc.C) {
	url := "local:/u/fred/prod/hosted-db2"

	tag := names.NewServiceTag("test")
	anOffer := crossmodel.NewOffer(tag, nil, url, nil)
	tag2 := names.NewServiceTag("testAgain")
	anOffer2 := crossmodel.NewOffer(tag2, nil, url, nil)

	crossmodel.TempPlaceholder[tag] = anOffer
	crossmodel.TempPlaceholder[tag2] = anOffer2

	found, err := s.api.Show(params.EndpointsSearchFilter{URL: url})
	c.Assert(err, gc.ErrorMatches, `expected to find one result for url "local:/u/fred/prod/hosted-db2" but found 2`)
	c.Assert(found, gc.DeepEquals, params.EndpointsDetailsResult{})
}

func (s *crossmodelSuite) TestShow(c *gc.C) {
	tag := names.NewServiceTag("test")
	url := "local:/u/fred/prod/hosted-db2"
	anOffer := crossmodel.NewOffer(tag, nil, url, nil)

	crossmodel.TempPlaceholder[tag] = anOffer

	found, err := s.api.Show(params.EndpointsSearchFilter{URL: url})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, gc.DeepEquals, params.EndpointsDetailsResult{Service: tag.String()})
}

func (s *crossmodelSuite) TestShowNotFound(c *gc.C) {
	url := "local:/u/fred/prod/hosted-db2"

	found, err := s.api.Show(params.EndpointsSearchFilter{URL: url})
	c.Assert(err, gc.ErrorMatches, `endpoints with url "local:/u/fred/prod/hosted-db2" not found`)
	c.Assert(found, gc.DeepEquals, params.EndpointsDetailsResult{})
}

func (s *crossmodelSuite) TestShowFoundMultiple(c *gc.C) {
	url := "local:/u/fred/prod/hosted-db2"

	tag := names.NewServiceTag("test")
	anOffer := crossmodel.NewOffer(tag, nil, url, nil)
	tag2 := names.NewServiceTag("testAgain")
	anOffer2 := crossmodel.NewOffer(tag2, nil, url, nil)

	crossmodel.TempPlaceholder[tag] = anOffer
	crossmodel.TempPlaceholder[tag2] = anOffer2

	found, err := s.api.Show(params.EndpointsSearchFilter{URL: url})
	c.Assert(err, gc.ErrorMatches, `expected to find one result for url "local:/u/fred/prod/hosted-db2" but found 2`)
	c.Assert(found, gc.DeepEquals, params.EndpointsDetailsResult{})
}
