// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodeloperator_test

import (
	"context"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/caasmodeloperator"
	"github.com/juju/juju/core/resource"
	"github.com/juju/juju/internal/version"
	"github.com/juju/juju/rpc/params"
)

type ModelOperatorSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ModelOperatorSuite{})

func (m *ModelOperatorSuite) TestProvisioningInfo(c *gc.C) {
	var (
		apiAddresses = []string{"fe80:abcd::1"}
		imagePath    = "juju/juju"
	)
	ver, err := version.Parse("1.2.3")
	c.Assert(err, jc.ErrorIsNil)

	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASModelOperator")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "ModelOperatorProvisioningInfo")
		c.Assert(result, gc.FitsTypeOf, &params.ModelOperatorInfo{})

		*(result.(*params.ModelOperatorInfo)) = params.ModelOperatorInfo{
			APIAddresses: apiAddresses,
			ImageDetails: params.DockerImageInfo{RegistryPath: imagePath},
			Version:      ver,
		}
		return nil
	})

	client := caasmodeloperator.NewClient(apiCaller)
	result, err := client.ModelOperatorProvisioningInfo(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(result.APIAddresses, jc.DeepEquals, apiAddresses)
	c.Assert(result.ImageDetails, jc.DeepEquals, resource.DockerImageDetails{RegistryPath: imagePath})
	c.Assert(result.Version, jc.DeepEquals, ver)
}

func (m *ModelOperatorSuite) TestSetPassword(c *gc.C) {
	var (
		called   = false
		password = "fee75f71b1b3ddf4e7996ce5ce8ad1a49ff16c9a10c025c7db2d8600d0921bb0"
	)
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, a, result interface{}) error {
		called = true
		c.Check(objType, gc.Equals, "CAASModelOperator")
		c.Check(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "SetPasswords")
		c.Assert(a, jc.DeepEquals, params.EntityPasswords{
			Changes: []params.EntityPassword{{Tag: "model-deadbeef-0bad-400d-8000-4b1d0d06f00d", Password: password}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{}},
		}
		return nil
	})

	client := caasmodeloperator.NewClient(apiCaller)
	err := client.SetPassword(context.Background(), password)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}
