// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager_test

import (
	stdtesting "testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/client/modelmanager"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type accessSuite struct {
	testing.BaseSuite
}

func TestAccessSuite(t *stdtesting.T) { tc.Run(t, &accessSuite{}) }

const (
	someModelUUID = "63f5e78f-2d21-4d0c-a5c1-73463f3443bf"
	someModelTag  = "model-" + someModelUUID
)

func accessCall(c *tc.C, client *modelmanager.Client, action params.ModelAction, user, access string, modelUUIDs ...string) error {
	switch action {
	case params.GrantModelAccess:
		return client.GrantModel(c.Context(), user, access, modelUUIDs...)
	case params.RevokeModelAccess:
		return client.RevokeModel(c.Context(), user, access, modelUUIDs...)
	default:
		panic(action)
	}
}

func (s *accessSuite) TestGrantModelReadOnlyUser(c *tc.C) {
	s.readOnlyUser(c, params.GrantModelAccess)
}

func (s *accessSuite) TestRevokeModelReadOnlyUser(c *tc.C) {
	s.readOnlyUser(c, params.RevokeModelAccess)
}

func checkCall(c *tc.C, objType string, id, request string) {
	c.Check(objType, tc.Equals, "ModelManager")
	c.Check(id, tc.Equals, "")
	c.Check(request, tc.Equals, "ModifyModelAccess")
}

func assertRequest(c *tc.C, a interface{}) params.ModifyModelAccessRequest {
	req, ok := a.(params.ModifyModelAccessRequest)
	c.Assert(ok, tc.IsTrue, tc.Commentf("wrong request type"))
	return req
}

func assertResponse(c *tc.C, result interface{}) *params.ErrorResults {
	resp, ok := result.(*params.ErrorResults)
	c.Assert(ok, tc.IsTrue, tc.Commentf("wrong response type"))
	return resp
}

func (s *accessSuite) readOnlyUser(c *tc.C, action params.ModelAction) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, a, result interface{}) error {
			checkCall(c, objType, id, request)

			req := assertRequest(c, a)
			c.Assert(req.Changes, tc.HasLen, 1)
			c.Assert(string(req.Changes[0].Action), tc.Equals, string(action))
			c.Assert(string(req.Changes[0].Access), tc.Equals, string(params.ModelReadAccess))
			c.Assert(req.Changes[0].ModelTag, tc.Equals, someModelTag)

			resp := assertResponse(c, result)
			*resp = params.ErrorResults{Results: []params.ErrorResult{{Error: nil}}}

			return nil
		})
	client := modelmanager.NewClient(apiCaller)
	err := accessCall(c, client, action, "bob", "read", someModelUUID)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *accessSuite) TestGrantModelAdminUser(c *tc.C) {
	s.adminUser(c, params.GrantModelAccess)
}

func (s *accessSuite) TestRevokeModelAdminUser(c *tc.C) {
	s.adminUser(c, params.RevokeModelAccess)
}

func (s *accessSuite) adminUser(c *tc.C, action params.ModelAction) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, a, result interface{}) error {
			checkCall(c, objType, id, request)

			req := assertRequest(c, a)
			c.Assert(req.Changes, tc.HasLen, 1)
			c.Assert(string(req.Changes[0].Action), tc.Equals, string(action))
			c.Assert(string(req.Changes[0].Access), tc.Equals, string(params.ModelWriteAccess))
			c.Assert(req.Changes[0].ModelTag, tc.Equals, someModelTag)

			resp := assertResponse(c, result)
			*resp = params.ErrorResults{Results: []params.ErrorResult{{Error: nil}}}

			return nil
		})
	client := modelmanager.NewClient(apiCaller)
	err := accessCall(c, client, action, "bob", "write", someModelUUID)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *accessSuite) TestGrantThreeModels(c *tc.C) {
	s.threeModels(c, params.GrantModelAccess)
}

func (s *accessSuite) TestRevokeThreeModels(c *tc.C) {
	s.threeModels(c, params.RevokeModelAccess)
}

func (s *accessSuite) threeModels(c *tc.C, action params.ModelAction) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, a, result interface{}) error {
			checkCall(c, objType, id, request)

			req := assertRequest(c, a)
			c.Assert(req.Changes, tc.HasLen, 3)
			for i := range req.Changes {
				c.Assert(string(req.Changes[i].Action), tc.Equals, string(action))
				c.Assert(string(req.Changes[i].Access), tc.Equals, string(params.ModelReadAccess))
				c.Assert(req.Changes[i].ModelTag, tc.Equals, someModelTag)
			}

			resp := assertResponse(c, result)
			*resp = params.ErrorResults{Results: []params.ErrorResult{{Error: nil}, {Error: nil}, {Error: nil}}}

			return nil
		})
	client := modelmanager.NewClient(apiCaller)
	err := accessCall(c, client, action, "carol", "read", someModelUUID, someModelUUID, someModelUUID)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *accessSuite) TestGrantErrorResult(c *tc.C) {
	s.errorResult(c, params.GrantModelAccess)
}

func (s *accessSuite) TestRevokeErrorResult(c *tc.C) {
	s.errorResult(c, params.RevokeModelAccess)
}

func (s *accessSuite) errorResult(c *tc.C, action params.ModelAction) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, a, result interface{}) error {
			checkCall(c, objType, id, request)

			req := assertRequest(c, a)
			c.Assert(req.Changes, tc.HasLen, 1)
			c.Assert(string(req.Changes[0].Action), tc.Equals, string(action))
			c.Assert(req.Changes[0].UserTag, tc.Equals, names.NewUserTag("aaa").String())
			c.Assert(req.Changes[0].ModelTag, tc.Equals, someModelTag)

			resp := assertResponse(c, result)
			err := &params.Error{Message: "unfortunate mishap"}
			*resp = params.ErrorResults{Results: []params.ErrorResult{{Error: err}}}

			return nil
		})
	client := modelmanager.NewClient(apiCaller)
	err := accessCall(c, client, action, "aaa", "write", someModelUUID)
	c.Assert(err, tc.ErrorMatches, "unfortunate mishap")
}

func (s *accessSuite) TestInvalidResultCount(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, a, result interface{}) error {
			checkCall(c, objType, id, request)
			assertRequest(c, a)

			resp := assertResponse(c, result)
			*resp = params.ErrorResults{Results: nil}

			return nil
		})
	client := modelmanager.NewClient(apiCaller)
	err := client.GrantModel(c.Context(), "bob", "write", someModelUUID, someModelUUID)
	c.Assert(err, tc.ErrorMatches, "expected 2 results, got 0")
}
