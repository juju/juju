// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/crossmodel"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/testing"
)

type crossmodelMockSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&crossmodelMockSuite{})

func (s *crossmodelMockSuite) TestOffer(c *gc.C) {
	service := "shared-fs/0"
	endPointA := "endPointA"
	endPointB := "endPointB"
	url := "url"
	user1 := "user1"
	user2 := "user2"

	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "CrossModel")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "Offer")

			args, ok := a.(params.CrossModelOffer)
			c.Assert(ok, jc.IsTrue)
			c.Assert(args.Service, gc.DeepEquals, service)
			c.Assert(args.Endpoints, jc.SameContents, []string{endPointA, endPointB})
			c.Assert(args.URL, gc.DeepEquals, url)
			c.Assert(args.Users, jc.SameContents, []string{user1, user2})
			return nil
		})

	client := crossmodel.NewClient(apiCaller)
	err := client.Offer(service, []string{endPointA, endPointB}, url, []string{user1, user2})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *crossmodelMockSuite) TestOfferFacadeCallError(c *gc.C) {
	msg := "facade failure"
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "CrossModel")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "Offer")

			return errors.New(msg)
		})
	client := crossmodel.NewClient(apiCaller)
	err := client.Offer("", nil, "", nil)
	c.Assert(errors.Cause(err), gc.ErrorMatches, msg)
}
