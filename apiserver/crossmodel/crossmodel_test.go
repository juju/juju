// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
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

	errs, err := s.api.Offer(all)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs.Results, gc.HasLen, len(all.Offers))
	c.Assert(errs.Results[0].Error, gc.IsNil)

	c.Assert(crossmodel.TempPlaceholder, gc.HasLen, 1)
	offer, err := apicrossmodel.ParseOffer(one)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(crossmodel.TempPlaceholder[serviceName], gc.DeepEquals, offer)
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
