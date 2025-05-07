// Copyright 2012-2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/controller"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/rpc/params"
)

func (s *Suite) TestIdentityProviderURLCallError(c *tc.C) {
	apiCaller := apitesting.BestVersionCaller{
		BestVersion: 7,
		APICallerFunc: func(string, int, string, string, interface{}, interface{}) error {
			return errors.New("boom")
		},
	}
	client := controller.NewClient(apiCaller)
	result, err := client.IdentityProviderURL(context.Background())
	c.Check(result, tc.Equals, "")
	c.Check(err, tc.ErrorMatches, "boom")
}

func (s *Suite) TestIdentityProviderURL(c *tc.C) {
	expURL := "https://api.jujucharms.com/identity"
	apiCaller := apitesting.BestVersionCaller{
		BestVersion: 7,
		APICallerFunc: func(objType string, version int, id, request string, arg, result interface{}) error {
			c.Check(objType, tc.Equals, "Controller")
			c.Check(id, tc.Equals, "")
			c.Check(request, tc.Equals, "IdentityProviderURL")
			c.Check(result, tc.FitsTypeOf, &params.StringResult{})

			out := result.(*params.StringResult)
			out.Result = expURL
			return nil
		},
	}

	client := controller.NewClient(apiCaller)
	result, err := client.IdentityProviderURL(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, tc.Equals, expURL)
}

func (s *Suite) TestIdentityProviderURLWithErrorResult(c *tc.C) {
	apiCaller := apitesting.BestVersionCaller{
		BestVersion: 7,
		APICallerFunc: func(objType string, version int, id, request string, arg, result interface{}) error {
			c.Check(objType, tc.Equals, "Controller")
			c.Check(id, tc.Equals, "")
			c.Check(request, tc.Equals, "IdentityProviderURL")
			c.Check(result, tc.FitsTypeOf, &params.StringResult{})

			out := result.(*params.StringResult)
			out.Result = "garbage"
			out.Error = apiservererrors.ServerError(errors.New("version error"))
			return nil
		},
	}

	client := controller.NewClient(apiCaller)
	_, err := client.IdentityProviderURL(context.Background())
	c.Assert(err, tc.ErrorMatches, "version error")
}
