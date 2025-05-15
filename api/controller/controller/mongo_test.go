// Copyright 2012-2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"github.com/juju/errors"
	"github.com/juju/tc"

	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/controller"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/rpc/params"
)

func (s *Suite) TestMongoVersionCallError(c *tc.C) {
	apiCaller := apitesting.BestVersionCaller{
		BestVersion: 6,
		APICallerFunc: func(string, int, string, string, interface{}, interface{}) error {
			return errors.New("boom")
		},
	}
	client := controller.NewClient(apiCaller)
	result, err := client.MongoVersion(c.Context())
	c.Check(result, tc.Equals, "")
	c.Check(err, tc.ErrorMatches, "boom")
}

func (s *Suite) TestMongoVersion(c *tc.C) {
	apiCaller := apitesting.BestVersionCaller{
		BestVersion: 6,
		APICallerFunc: func(objType string, version int, id, request string, arg, result interface{}) error {
			c.Check(objType, tc.Equals, "Controller")
			c.Check(id, tc.Equals, "")
			c.Check(request, tc.Equals, "MongoVersion")
			c.Check(result, tc.FitsTypeOf, &params.StringResult{})

			out := result.(*params.StringResult)
			out.Result = "3.5.12"
			return nil
		},
	}

	client := controller.NewClient(apiCaller)
	result, err := client.MongoVersion(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.Equals, "3.5.12")
}

func (s *Suite) TestMongoVersionWithErrorResult(c *tc.C) {
	apiCaller := apitesting.BestVersionCaller{
		BestVersion: 6,
		APICallerFunc: func(objType string, version int, id, request string, arg, result interface{}) error {
			c.Check(objType, tc.Equals, "Controller")
			c.Check(id, tc.Equals, "")
			c.Check(request, tc.Equals, "MongoVersion")
			c.Check(result, tc.FitsTypeOf, &params.StringResult{})

			out := result.(*params.StringResult)
			out.Result = "3.5.12"
			out.Error = apiservererrors.ServerError(errors.New("version error"))
			return nil
		},
	}

	client := controller.NewClient(apiCaller)
	_, err := client.MongoVersion(c.Context())
	c.Assert(err, tc.ErrorMatches, "version error")
}
