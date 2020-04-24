// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers_test

import (
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/applicationoffers"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/testing"
)

type accessSuite struct {
	testing.BaseSuite
}

type accessFunc func(string, string, ...string) error

var _ = gc.Suite(&accessSuite{})

const (
	someOffer = "user/prod.hosted-mysql"
)

func accessCall(client *applicationoffers.Client, action params.OfferAction, user, access string, offerURLs ...string) error {
	switch action {
	case params.GrantOfferAccess:
		return client.GrantOffer(user, access, offerURLs...)
	case params.RevokeOfferAccess:
		return client.RevokeOffer(user, access, offerURLs...)
	default:
		panic(action)
	}
}

func (s *accessSuite) TestGrantOfferReadOnlyUser(c *gc.C) {
	s.readOnlyUser(c, params.GrantOfferAccess)
}

func (s *accessSuite) TestRevokeOfferReadOnlyUser(c *gc.C) {
	s.readOnlyUser(c, params.RevokeOfferAccess)
}

func checkCall(c *gc.C, objType string, id, request string) {
	c.Check(objType, gc.Equals, "ApplicationOffers")
	c.Check(id, gc.Equals, "")
	c.Check(request, gc.Equals, "ModifyOfferAccess")
}

func assertRequest(c *gc.C, a interface{}) params.ModifyOfferAccessRequest {
	req, ok := a.(params.ModifyOfferAccessRequest)
	c.Assert(ok, jc.IsTrue, gc.Commentf("wrong request type"))
	return req
}

func assertResponse(c *gc.C, result interface{}) *params.ErrorResults {
	resp, ok := result.(*params.ErrorResults)
	c.Assert(ok, jc.IsTrue, gc.Commentf("wrong response type"))
	return resp
}

func (s *accessSuite) readOnlyUser(c *gc.C, action params.OfferAction) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, a, result interface{}) error {
			checkCall(c, objType, id, request)

			req := assertRequest(c, a)
			c.Assert(req.Changes, gc.HasLen, 1)
			c.Assert(string(req.Changes[0].Action), gc.Equals, string(action))
			c.Assert(string(req.Changes[0].Access), gc.Equals, string(params.OfferReadAccess))
			c.Assert(req.Changes[0].OfferURL, gc.Equals, someOffer)

			resp := assertResponse(c, result)
			*resp = params.ErrorResults{Results: []params.ErrorResult{{Error: nil}}}

			return nil
		})
	client := applicationoffers.NewClient(apiCaller)
	err := accessCall(client, action, "bob", "read", someOffer)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *accessSuite) TestGrantOfferAdminUser(c *gc.C) {
	s.adminUser(c, params.GrantOfferAccess)
}

func (s *accessSuite) TestRevokeOfferAdminUser(c *gc.C) {
	s.adminUser(c, params.RevokeOfferAccess)
}

func (s *accessSuite) adminUser(c *gc.C, action params.OfferAction) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, a, result interface{}) error {
			checkCall(c, objType, id, request)

			req := assertRequest(c, a)
			c.Assert(req.Changes, gc.HasLen, 1)
			c.Assert(string(req.Changes[0].Action), gc.Equals, string(action))
			c.Assert(string(req.Changes[0].Access), gc.Equals, string(params.OfferConsumeAccess))
			c.Assert(req.Changes[0].OfferURL, gc.Equals, someOffer)

			resp := assertResponse(c, result)
			*resp = params.ErrorResults{Results: []params.ErrorResult{{Error: nil}}}

			return nil
		})
	client := applicationoffers.NewClient(apiCaller)
	err := accessCall(client, action, "bob", "consume", someOffer)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *accessSuite) TestGrantThreeOffers(c *gc.C) {
	s.threeOffers(c, params.GrantOfferAccess)
}

func (s *accessSuite) TestRevokeThreeOffers(c *gc.C) {
	s.threeOffers(c, params.RevokeOfferAccess)
}

func (s *accessSuite) threeOffers(c *gc.C, action params.OfferAction) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, a, result interface{}) error {
			checkCall(c, objType, id, request)

			req := assertRequest(c, a)
			c.Assert(req.Changes, gc.HasLen, 3)
			for i := range req.Changes {
				c.Assert(string(req.Changes[i].Action), gc.Equals, string(action))
				c.Assert(string(req.Changes[i].Access), gc.Equals, string(params.OfferReadAccess))
				c.Assert(req.Changes[i].OfferURL, gc.Equals, someOffer)
			}

			resp := assertResponse(c, result)
			*resp = params.ErrorResults{Results: []params.ErrorResult{{Error: nil}, {Error: nil}, {Error: nil}}}

			return nil
		})
	client := applicationoffers.NewClient(apiCaller)
	err := accessCall(client, action, "carol", "read", someOffer, someOffer, someOffer)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *accessSuite) TestGrantErrorResult(c *gc.C) {
	s.errorResult(c, params.GrantOfferAccess)
}

func (s *accessSuite) TestRevokeErrorResult(c *gc.C) {
	s.errorResult(c, params.RevokeOfferAccess)
}

func (s *accessSuite) errorResult(c *gc.C, action params.OfferAction) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, a, result interface{}) error {
			checkCall(c, objType, id, request)

			req := assertRequest(c, a)
			c.Assert(req.Changes, gc.HasLen, 1)
			c.Assert(string(req.Changes[0].Action), gc.Equals, string(action))
			c.Assert(req.Changes[0].UserTag, gc.Equals, names.NewUserTag("aaa").String())
			c.Assert(req.Changes[0].OfferURL, gc.Equals, someOffer)

			resp := assertResponse(c, result)
			err := &params.Error{Message: "unfortunate mishap"}
			*resp = params.ErrorResults{Results: []params.ErrorResult{{Error: err}}}

			return nil
		})
	client := applicationoffers.NewClient(apiCaller)
	err := accessCall(client, action, "aaa", "consume", someOffer)
	c.Assert(err, gc.ErrorMatches, "unfortunate mishap")
}

func (s *accessSuite) TestInvalidResultCount(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, a, result interface{}) error {
			checkCall(c, objType, id, request)
			assertRequest(c, a)

			resp := assertResponse(c, result)
			*resp = params.ErrorResults{Results: nil}

			return nil
		})
	client := applicationoffers.NewClient(apiCaller)
	err := client.GrantOffer("bob", "consume", someOffer, someOffer)
	c.Assert(err, gc.ErrorMatches, "expected 2 results, got 0")
}
