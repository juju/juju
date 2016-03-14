// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager_test

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/modelmanager"
	"github.com/juju/juju/apiserver/params"
	jujutesting "github.com/juju/juju/juju/testing"
)

type accessSuite struct {
	jujutesting.JujuConnSuite

	modelmanager *modelmanager.Client
}

type accessFunc func(string, string, ...string) error

var _ = gc.Suite(&accessSuite{})

const (
	someModelUUID = "63f5e78f-2d21-4d0c-a5c1-73463f3443bf"
	someModelTag  = "model-" + someModelUUID
)

func (s *accessSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.modelmanager = modelmanager.NewClient(s.APIState)
	c.Assert(s.modelmanager, gc.NotNil)
}

func (s *accessSuite) accessFunc(action params.ModelAction) accessFunc {
	switch action {
	case params.GrantModelAccess:
		return s.modelmanager.GrantModel
	case params.RevokeModelAccess:
		return s.modelmanager.RevokeModel
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

func (s *accessSuite) readOnlyUser(c *gc.C, action params.ModelAction) {
	modelmanager.PatchFacadeCall(s, s.modelmanager, func(request string, paramsIn interface{}, response interface{}) error {
		if req, ok := paramsIn.(params.ModifyModelAccessRequest); ok {
			c.Assert(req.Changes, gc.HasLen, 1)
			c.Assert(string(req.Changes[0].Action), gc.Equals, string(action))
			c.Assert(string(req.Changes[0].Access), gc.Equals, string(params.ModelReadAccess))
			c.Assert(req.Changes[0].ModelTag, gc.Equals, someModelTag)
		} else {
			c.Fatalf("wrong input structure")
		}
		if result, ok := response.(*params.ErrorResults); ok {
			*result = params.ErrorResults{Results: []params.ErrorResult{{Error: nil}}}
		} else {
			c.Fatalf("wrong input structure")
		}
		return nil
	})

	fn := s.accessFunc(action)
	err := fn("bob", "read", someModelUUID)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *accessSuite) TestGrantModelAdminUser(c *gc.C) {
	s.adminUser(c, params.GrantModelAccess)
}

func (s *accessSuite) TestRevokeModelAdminUser(c *gc.C) {
	s.adminUser(c, params.RevokeModelAccess)
}

func (s *accessSuite) adminUser(c *gc.C, action params.ModelAction) {
	modelmanager.PatchFacadeCall(s, s.modelmanager, func(request string, paramsIn interface{}, response interface{}) error {
		if req, ok := paramsIn.(params.ModifyModelAccessRequest); ok {
			c.Assert(req.Changes, gc.HasLen, 1)
			c.Assert(string(req.Changes[0].Action), gc.Equals, string(action))
			c.Assert(string(req.Changes[0].Access), gc.Equals, string(params.ModelWriteAccess))
			c.Assert(req.Changes[0].ModelTag, gc.Equals, someModelTag)
		} else {
			c.Fatalf("wrong input structure")
		}
		if result, ok := response.(*params.ErrorResults); ok {
			*result = params.ErrorResults{Results: []params.ErrorResult{{Error: nil}}}
		} else {
			c.Fatalf("wrong input structure")
		}
		return nil
	})

	fn := s.accessFunc(action)
	err := fn(s.Factory.MakeModelUser(c, nil).UserTag().Name(), "write", someModelUUID)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *accessSuite) TestGrantThreeModels(c *gc.C) {
	s.threeModels(c, params.GrantModelAccess)
}

func (s *accessSuite) TestRevokeThreeModels(c *gc.C) {
	s.threeModels(c, params.RevokeModelAccess)
}

func (s *accessSuite) threeModels(c *gc.C, action params.ModelAction) {
	modelmanager.PatchFacadeCall(s, s.modelmanager, func(request string, paramsIn interface{}, response interface{}) error {
		if req, ok := paramsIn.(params.ModifyModelAccessRequest); ok {
			c.Assert(req.Changes, gc.HasLen, 3)
			for i := range req.Changes {
				c.Assert(string(req.Changes[i].Action), gc.Equals, string(action))
				c.Assert(string(req.Changes[i].Access), gc.Equals, string(params.ModelReadAccess))
				c.Assert(req.Changes[i].ModelTag, gc.Equals, someModelTag)
			}
		} else {
			c.Log("wrong input structure")
			c.Fail()
		}
		if result, ok := response.(*params.ErrorResults); ok {
			*result = params.ErrorResults{Results: []params.ErrorResult{{Error: nil}}}
		} else {
			c.Log("wrong output structure")
			c.Fail()
		}
		return nil
	})

	fn := s.accessFunc(action)
	err := fn(s.Factory.MakeModelUser(c, nil).UserTag().Name(), "read",
		someModelUUID, someModelUUID, someModelUUID)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *accessSuite) TestGrantErrorResult(c *gc.C) {
	s.errorResult(c, params.GrantModelAccess)
}

func (s *accessSuite) TestRevokeErrorResult(c *gc.C) {
	s.errorResult(c, params.RevokeModelAccess)
}

func (s *accessSuite) errorResult(c *gc.C, action params.ModelAction) {
	modelmanager.PatchFacadeCall(s, s.modelmanager, func(request string, paramsIn interface{}, response interface{}) error {
		if req, ok := paramsIn.(params.ModifyModelAccessRequest); ok {
			c.Assert(req.Changes, gc.HasLen, 1)
			c.Assert(string(req.Changes[0].Action), gc.Equals, string(action))
			c.Assert(req.Changes[0].UserTag, gc.Equals, names.NewUserTag("aaa").String())
			c.Assert(req.Changes[0].ModelTag, gc.Equals, someModelTag)
		} else {
			c.Log("wrong input structure")
			c.Fail()
		}
		if result, ok := response.(*params.ErrorResults); ok {
			err := &params.Error{Message: "unfortunate mishap"}
			*result = params.ErrorResults{Results: []params.ErrorResult{{Error: err}, {Error: nil}, {Error: nil}}}
		} else {
			c.Log("wrong output structure")
			c.Fail()
		}
		return nil
	})

	fn := s.accessFunc(action)
	err := fn("aaa", "write", someModelUUID)
	c.Assert(err, gc.ErrorMatches, "unfortunate mishap")
}
