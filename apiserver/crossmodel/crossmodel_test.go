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
