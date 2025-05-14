// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodeloperator_test

import (
	"github.com/juju/tc"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/caasmodeloperator"
	"github.com/juju/juju/core/resource"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

type ModelOperatorSuite struct {
	testhelpers.IsolationSuite
}

var _ = tc.Suite(&ModelOperatorSuite{})

func (m *ModelOperatorSuite) TestProvisioningInfo(c *tc.C) {
	var (
		apiAddresses = []string{"fe80:abcd::1"}
		imagePath    = "juju/juju"
	)
	ver, err := semversion.Parse("1.2.3")
	c.Assert(err, tc.ErrorIsNil)

	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "CAASModelOperator")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "ModelOperatorProvisioningInfo")
		c.Assert(result, tc.FitsTypeOf, &params.ModelOperatorInfo{})

		*(result.(*params.ModelOperatorInfo)) = params.ModelOperatorInfo{
			APIAddresses: apiAddresses,
			ImageDetails: params.DockerImageInfo{RegistryPath: imagePath},
			Version:      ver,
		}
		return nil
	})

	client := caasmodeloperator.NewClient(apiCaller)
	result, err := client.ModelOperatorProvisioningInfo(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(result.APIAddresses, tc.DeepEquals, apiAddresses)
	c.Assert(result.ImageDetails, tc.DeepEquals, resource.DockerImageDetails{RegistryPath: imagePath})
	c.Assert(result.Version, tc.DeepEquals, ver)
}

func (m *ModelOperatorSuite) TestSetPassword(c *tc.C) {
	var (
		called   = false
		password = "fee75f71b1b3ddf4e7996ce5ce8ad1a49ff16c9a10c025c7db2d8600d0921bb0"
	)
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, a, result interface{}) error {
		called = true
		c.Check(objType, tc.Equals, "CAASModelOperator")
		c.Check(id, tc.Equals, "")
		c.Assert(request, tc.Equals, "SetPasswords")
		c.Assert(a, tc.DeepEquals, params.EntityPasswords{
			Changes: []params.EntityPassword{{Tag: "model-deadbeef-0bad-400d-8000-4b1d0d06f00d", Password: password}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{}},
		}
		return nil
	})

	client := caasmodeloperator.NewClient(apiCaller)
	err := client.SetPassword(c.Context(), password)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(called, tc.IsTrue)
}
