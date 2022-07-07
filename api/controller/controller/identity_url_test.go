// Copyright 2012-2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/controller"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/rpc/params"
)

func (s *Suite) TestIdentityProviderURLCallError(c *gc.C) {
	apiCaller := apitesting.BestVersionCaller{
		BestVersion: 7,
		APICallerFunc: func(string, int, string, string, interface{}, interface{}) error {
			return errors.New("boom")
		},
	}
	client := controller.NewClient(apiCaller)
	result, err := client.IdentityProviderURL()
	c.Check(result, gc.Equals, "")
	c.Check(err, gc.ErrorMatches, "boom")
}

func (s *Suite) TestIdentityProviderURL(c *gc.C) {
	expURL := "https://api.jujucharms.com/identity"
	apiCaller := apitesting.BestVersionCaller{
		BestVersion: 7,
		APICallerFunc: func(objType string, version int, id, request string, arg, result interface{}) error {
			c.Check(objType, gc.Equals, "Controller")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "IdentityProviderURL")
			c.Check(result, gc.FitsTypeOf, &params.StringResult{})

			out := result.(*params.StringResult)
			out.Result = expURL
			return nil
		},
	}

	client := controller.NewClient(apiCaller)
	result, err := client.IdentityProviderURL()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.Equals, expURL)
}

func (s *Suite) TestIdentityProviderURLWithErrorResult(c *gc.C) {
	apiCaller := apitesting.BestVersionCaller{
		BestVersion: 7,
		APICallerFunc: func(objType string, version int, id, request string, arg, result interface{}) error {
			c.Check(objType, gc.Equals, "Controller")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "IdentityProviderURL")
			c.Check(result, gc.FitsTypeOf, &params.StringResult{})

			out := result.(*params.StringResult)
			out.Result = "garbage"
			out.Error = apiservererrors.ServerError(errors.New("version error"))
			return nil
		},
	}

	client := controller.NewClient(apiCaller)
	_, err := client.IdentityProviderURL()
	c.Assert(err, gc.ErrorMatches, "version error")
}
