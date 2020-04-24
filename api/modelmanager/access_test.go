// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager_test

import (
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/modelmanager"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/testing"
)

type accessSuite struct {
	testing.BaseSuite
}

type accessFunc func(string, string, ...string) error

var _ = gc.Suite(&accessSuite{})

const (
	someModelUUID = "63f5e78f-2d21-4d0c-a5c1-73463f3443bf"
	someModelTag  = "model-" + someModelUUID
)

func accessCall(client *modelmanager.Client, action params.ModelAction, user, access string, modelUUIDs ...string) error {
	switch action {
	case params.GrantModelAccess:
		return client.GrantModel(user, access, modelUUIDs...)
	case params.RevokeModelAccess:
		return client.RevokeModel(user, access, modelUUIDs...)
	default:
		panic(action)
	}
}

func (s *accessSuite) TestGrantModelReadOnlyUser(c *gc.C) {
	s.readOnlyUser(c, params.GrantModelAccess)
}

func (s *accessSuite) TestRevokeModelReadOnlyUser(c *gc.C) {
	s.readOnlyUser(c, params.RevokeModelAccess)
}

func checkCall(c *gc.C, objType string, id, request string) {
	c.Check(objType, gc.Equals, "ModelManager")
	c.Check(id, gc.Equals, "")
	c.Check(request, gc.Equals, "ModifyModelAccess")
}

func assertRequest(c *gc.C, a interface{}) params.ModifyModelAccessRequest {
	req, ok := a.(params.ModifyModelAccessRequest)
	c.Assert(ok, jc.IsTrue, gc.Commentf("wrong request type"))
	return req
}

func assertResponse(c *gc.C, result interface{}) *params.ErrorResults {
	resp, ok := result.(*params.ErrorResults)
	c.Assert(ok, jc.IsTrue, gc.Commentf("wrong response type"))
	return resp
}

func (s *accessSuite) readOnlyUser(c *gc.C, action params.ModelAction) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, a, result interface{}) error {
			checkCall(c, objType, id, request)

			req := assertRequest(c, a)
			c.Assert(req.Changes, gc.HasLen, 1)
			c.Assert(string(req.Changes[0].Action), gc.Equals, string(action))
			c.Assert(string(req.Changes[0].Access), gc.Equals, string(params.ModelReadAccess))
			c.Assert(req.Changes[0].ModelTag, gc.Equals, someModelTag)

			resp := assertResponse(c, result)
			*resp = params.ErrorResults{Results: []params.ErrorResult{{Error: nil}}}

			return nil
		})
	client := modelmanager.NewClient(apiCaller)
	err := accessCall(client, action, "bob", "read", someModelUUID)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *accessSuite) TestGrantModelAdminUser(c *gc.C) {
	s.adminUser(c, params.GrantModelAccess)
}

func (s *accessSuite) TestRevokeModelAdminUser(c *gc.C) {
	s.adminUser(c, params.RevokeModelAccess)
}

func (s *accessSuite) adminUser(c *gc.C, action params.ModelAction) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, a, result interface{}) error {
			checkCall(c, objType, id, request)

			req := assertRequest(c, a)
			c.Assert(req.Changes, gc.HasLen, 1)
			c.Assert(string(req.Changes[0].Action), gc.Equals, string(action))
			c.Assert(string(req.Changes[0].Access), gc.Equals, string(params.ModelWriteAccess))
			c.Assert(req.Changes[0].ModelTag, gc.Equals, someModelTag)

			resp := assertResponse(c, result)
			*resp = params.ErrorResults{Results: []params.ErrorResult{{Error: nil}}}

			return nil
		})
	client := modelmanager.NewClient(apiCaller)
	err := accessCall(client, action, "bob", "write", someModelUUID)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *accessSuite) TestGrantThreeModels(c *gc.C) {
	s.threeModels(c, params.GrantModelAccess)
}

func (s *accessSuite) TestRevokeThreeModels(c *gc.C) {
	s.threeModels(c, params.RevokeModelAccess)
}

func (s *accessSuite) threeModels(c *gc.C, action params.ModelAction) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, a, result interface{}) error {
			checkCall(c, objType, id, request)

			req := assertRequest(c, a)
			c.Assert(req.Changes, gc.HasLen, 3)
			for i := range req.Changes {
				c.Assert(string(req.Changes[i].Action), gc.Equals, string(action))
				c.Assert(string(req.Changes[i].Access), gc.Equals, string(params.ModelReadAccess))
				c.Assert(req.Changes[i].ModelTag, gc.Equals, someModelTag)
			}

			resp := assertResponse(c, result)
			*resp = params.ErrorResults{Results: []params.ErrorResult{{Error: nil}, {Error: nil}, {Error: nil}}}

			return nil
		})
	client := modelmanager.NewClient(apiCaller)
	err := accessCall(client, action, "carol", "read", someModelUUID, someModelUUID, someModelUUID)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *accessSuite) TestGrantErrorResult(c *gc.C) {
	s.errorResult(c, params.GrantModelAccess)
}

func (s *accessSuite) TestRevokeErrorResult(c *gc.C) {
	s.errorResult(c, params.RevokeModelAccess)
}

func (s *accessSuite) errorResult(c *gc.C, action params.ModelAction) {
	apiCaller := basetesting.APICallerFunc(
		func(objType string, version int, id, request string, a, result interface{}) error {
			checkCall(c, objType, id, request)

			req := assertRequest(c, a)
			c.Assert(req.Changes, gc.HasLen, 1)
			c.Assert(string(req.Changes[0].Action), gc.Equals, string(action))
			c.Assert(req.Changes[0].UserTag, gc.Equals, names.NewUserTag("aaa").String())
			c.Assert(req.Changes[0].ModelTag, gc.Equals, someModelTag)

			resp := assertResponse(c, result)
			err := &params.Error{Message: "unfortunate mishap"}
			*resp = params.ErrorResults{Results: []params.ErrorResult{{Error: err}}}

			return nil
		})
	client := modelmanager.NewClient(apiCaller)
	err := accessCall(client, action, "aaa", "write", someModelUUID)
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
	client := modelmanager.NewClient(apiCaller)
	err := client.GrantModel("bob", "write", someModelUUID, someModelUUID)
	c.Assert(err, gc.ErrorMatches, "expected 2 results, got 0")
}
