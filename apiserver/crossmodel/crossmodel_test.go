// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apicrossmodel "github.com/juju/juju/apiserver/crossmodel"
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
	serviceName := names.NewServiceTag("test")
	one := params.CrossModelOffer{serviceName.String(), nil, "", nil}
	all := params.CrossModelOffers{[]params.CrossModelOffer{one}}

	var stored crossmodel.Offer
	s.exporter.exportOffer = func(offer crossmodel.Offer) error {
		s.calls = append(s.calls, exportOfferCall)
		stored = offer
		return nil
	}

	errs, err := s.api.Offer(all)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs.Results, gc.HasLen, len(all.Offers))
	c.Assert(errs.Results[0].Error, gc.IsNil)

	offer, err := apicrossmodel.ParseOffer(one)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stored, gc.DeepEquals, offer)
	s.assertCalls(c, exportOfferCall)
}

func (s *crossmodelSuite) TestOfferError(c *gc.C) {
	serviceName := names.NewServiceTag("test")
	one := params.CrossModelOffer{serviceName.String(), nil, "", nil}
	all := params.CrossModelOffers{[]params.CrossModelOffer{one}}

	msg := "fail"

	s.exporter.exportOffer = func(offer crossmodel.Offer) error {
		s.calls = append(s.calls, exportOfferCall)
		return errors.New(msg)
	}

	errs, err := s.api.Offer(all)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs.Results, gc.HasLen, len(all.Offers))
	c.Assert(errs.Results[0].Error, gc.ErrorMatches, fmt.Sprintf(".*%v.*", msg))
	s.assertCalls(c, exportOfferCall)
}

func (s *crossmodelSuite) TestListOffers(c *gc.C) {
	tag := names.NewServiceTag("test")
	url := "local:/u/fred/prod/hosted-db2"
	anOffer := crossmodel.RemoteService{Service: tag, URL: url}

	s.exporter.search = func(urls []string) ([]crossmodel.RemoteServiceEndpoints, error) {
		s.calls = append(s.calls, searchCall)
		return []crossmodel.RemoteServiceEndpoints{
			crossmodel.RemoteServiceEndpoints{anOffer, nil},
		}, nil
	}

	found, err := s.api.Show([]string{url})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, gc.DeepEquals,
		params.RemoteServiceInfoResults{[]params.RemoteServiceInfoResult{
			{Result: params.RemoteServiceInfo{Service: tag.String(), Endpoints: []params.RemoteEndpoint{}}},
		}})
	s.assertCalls(c, searchCall)
}

func (s *crossmodelSuite) TestListOffersError(c *gc.C) {
	url := "local:/u/fred/prod/hosted-db2"
	msg := "fail"

	s.exporter.search = func(urls []string) ([]crossmodel.RemoteServiceEndpoints, error) {
		s.calls = append(s.calls, searchCall)
		return nil, errors.New(msg)
	}

	found, err := s.api.Show([]string{url})
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf(".*%v.*", msg))
	c.Assert(found.Results, gc.HasLen, 0)
	s.assertCalls(c, searchCall)
}

func (s *crossmodelSuite) TestListOffersNotFound(c *gc.C) {
	urls := []string{"local:/u/fred/prod/hosted-db2"}

	s.exporter.search = func(urls []string) ([]crossmodel.RemoteServiceEndpoints, error) {
		s.calls = append(s.calls, searchCall)
		return nil, nil
	}

	found, err := s.api.Show(urls)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf(`endpoints with urls %v not found`, strings.Join(urls, ",")))
	c.Assert(found.Results, gc.HasLen, 0)
	s.assertCalls(c, searchCall)
}

func (s *crossmodelSuite) TestListOffersErrorMsgMultipleURLs(c *gc.C) {
	urls := []string{"local:/u/fred/prod/hosted-db2", "remote:/u/fred/hosted-db2"}

	s.exporter.search = func(urls []string) ([]crossmodel.RemoteServiceEndpoints, error) {
		s.calls = append(s.calls, searchCall)
		return nil, nil
	}

	found, err := s.api.Show(urls)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf(`endpoints with urls %v not found`, strings.Join(urls, ",")))
	c.Assert(found.Results, gc.HasLen, 0)
	s.assertCalls(c, searchCall)
}

func (s *crossmodelSuite) TestListOffersNotFoundEmpty(c *gc.C) {
	urls := []string{"local:/u/fred/prod/hosted-db2"}

	s.exporter.search = func(urls []string) ([]crossmodel.RemoteServiceEndpoints, error) {
		s.calls = append(s.calls, searchCall)
		return []crossmodel.RemoteServiceEndpoints{}, nil
	}

	found, err := s.api.Show(urls)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf(`endpoints with urls %v not found`, strings.Join(urls, ",")))
	c.Assert(found.Results, gc.HasLen, 0)
	s.assertCalls(c, searchCall)
}

func (s *crossmodelSuite) TestListOffersFoundMultiple(c *gc.C) {
	url := "local:/u/fred/prod/hosted-db2"

	tag := names.NewServiceTag("test")
	anOffer := crossmodel.RemoteService{Service: tag, URL: url}
	tag2 := names.NewServiceTag("testAgain")
	anOffer2 := crossmodel.RemoteService{Service: tag2, URL: url}

	s.exporter.search = func(urls []string) ([]crossmodel.RemoteServiceEndpoints, error) {
		s.calls = append(s.calls, searchCall)
		return []crossmodel.RemoteServiceEndpoints{
			crossmodel.RemoteServiceEndpoints{anOffer, nil},
			crossmodel.RemoteServiceEndpoints{anOffer2, nil},
		}, nil
	}

	found, err := s.api.Show([]string{url})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, gc.DeepEquals, params.RemoteServiceInfoResults{
		[]params.RemoteServiceInfoResult{
			{Result: params.RemoteServiceInfo{Service: tag.String(), Endpoints: []params.RemoteEndpoint{}}},
			{Result: params.RemoteServiceInfo{Service: tag2.String(), Endpoints: []params.RemoteEndpoint{}}},
		}})
	s.assertCalls(c, searchCall)
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
