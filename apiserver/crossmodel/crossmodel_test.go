// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"fmt"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
)

type crossmodelSuite struct {
	baseCrossmodelSuite
}

var _ = gc.Suite(&crossmodelSuite{})

func (s *crossmodelSuite) TestOffer(c *gc.C) {
	all := make(map[string]params.CrossModelOffer)
	s.state.offer = func(o params.CrossModelOffer) error {
		s.calls = append(s.calls, offerCall)
		all[o.Service] = o
		return nil
	}

	serviceName := "test"
	offer := params.CrossModelOffer{serviceName, nil, "", nil}

	err := s.api.Offer(offer)
	c.Assert(err, jc.ErrorIsNil)
	s.assertCalls(c, []string{offerCall})
	c.Assert(all, gc.HasLen, 1)
	c.Assert(all[serviceName], gc.DeepEquals, offer)
}

func (s *crossmodelSuite) TestOfferError(c *gc.C) {
	msg := "fail offer"
	s.state.offer = func(o params.CrossModelOffer) error {
		s.calls = append(s.calls, offerCall)
		return errors.New(msg)
	}
	serviceName := "test"
	offer := params.CrossModelOffer{serviceName, nil, "", nil}

	err := s.api.Offer(offer)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf(".*%v.*", msg))
	s.assertCalls(c, []string{offerCall})
}
