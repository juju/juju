// Copyright 2012-2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/params"
)

func (s *Suite) TestMongoVersionPriorV6(c *gc.C) {
	called := false
	apiCaller := apitesting.BestVersionCaller{
		BestVersion: 5,
		APICallerFunc: func(objType string, version int, id, request string, a, response interface{}) error {
			called = true
			c.Assert(request, gc.Equals, "MongoVersion")
			return nil
		},
	}

	client := controller.NewClient(apiCaller)
	_, err := client.MongoVersion()
	c.Assert(err, gc.ErrorMatches, "MongoVersion not supported by this version of Juju not supported")
	c.Assert(called, jc.IsFalse)
}

func (s *Suite) TestMongoVersionCallError(c *gc.C) {
	apiCaller := apitesting.BestVersionCaller{
		BestVersion: 6,
		APICallerFunc: func(string, int, string, string, interface{}, interface{}) error {
			return errors.New("boom")
		},
	}
	client := controller.NewClient(apiCaller)
	result, err := client.MongoVersion()
	c.Check(result, gc.Equals, "")
	c.Check(err, gc.ErrorMatches, "boom")
}

func (s *Suite) TestMongoVersion(c *gc.C) {
	apiCaller := apitesting.BestVersionCaller{
		BestVersion: 6,
		APICallerFunc: func(objType string, version int, id, request string, arg, result interface{}) error {
			c.Check(objType, gc.Equals, "Controller")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "MongoVersion")
			c.Check(result, gc.FitsTypeOf, &params.StringResult{})

			out := result.(*params.StringResult)
			out.Result = "3.5.12"
			return nil
		},
	}

	client := controller.NewClient(apiCaller)
	result, err := client.MongoVersion()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.Equals, "3.5.12")
}

func (s *Suite) TestMongoVersionWithErrorResult(c *gc.C) {
	apiCaller := apitesting.BestVersionCaller{
		BestVersion: 6,
		APICallerFunc: func(objType string, version int, id, request string, arg, result interface{}) error {
			c.Check(objType, gc.Equals, "Controller")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "MongoVersion")
			c.Check(result, gc.FitsTypeOf, &params.StringResult{})

			out := result.(*params.StringResult)
			out.Result = "3.5.12"
			out.Error = apiservererrors.ServerError(errors.New("version error"))
			return nil
		},
	}

	client := controller.NewClient(apiCaller)
	_, err := client.MongoVersion()
	c.Assert(err, gc.ErrorMatches, "version error")
}
