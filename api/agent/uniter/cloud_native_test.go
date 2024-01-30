// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"context"

	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

type cloudNativeUniterSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&cloudNativeUniterSuite{})

func (s *cloudNativeUniterSuite) TestCloudSpec(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(request, gc.Equals, "CloudSpec")
		c.Assert(arg, gc.IsNil)
		c.Assert(result, gc.FitsTypeOf, &params.CloudSpecResult{})
		*(result.(*params.CloudSpecResult)) = params.CloudSpecResult{
			Result: &params.CloudSpec{
				Name: "dummy",
				Credential: &params.CloudCredential{
					Attributes: map[string]string{
						"username": "dummy",
						"password": "secret",
					},
				},
			},
		}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("wordpress/0"))

	result, err := client.CloudSpec(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Name, gc.Equals, "dummy")
	c.Assert(result.Credential.Attributes, gc.DeepEquals, map[string]string{
		"username": "dummy",
		"password": "secret",
	})
}
