// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner_test

import (
	"testing"

	"github.com/juju/tc"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/caasapplicationprovisioner"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

type provisionerSuite struct {
	testhelpers.IsolationSuite
}

func TestProvisionerSuite(t *testing.T) {
	tc.Run(t, &provisionerSuite{})
}

func newClient(f basetesting.APICallerFunc) *caasapplicationprovisioner.Client {
	return caasapplicationprovisioner.NewClient(basetesting.BestVersionCaller{APICallerFunc: f, BestVersion: 1})
}

func (s *provisionerSuite) TestProvisioningInfo(c *tc.C) {
	vers := semversion.MustParse("2.99.0")
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		c.Check(objType, tc.Equals, "CAASApplicationProvisioner")
		c.Check(id, tc.Equals, "")
		c.Assert(request, tc.Equals, "ProvisioningInfo")
		c.Assert(a, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "application-gitlab"}}})
		c.Assert(result, tc.FitsTypeOf, &params.CAASApplicationProvisioningInfoResults{})
		*(result.(*params.CAASApplicationProvisioningInfoResults)) = params.CAASApplicationProvisioningInfoResults{
			Results: []params.CAASApplicationProvisioningInfo{{
				Version:      vers,
				APIAddresses: []string{"10.0.0.1:1"},
				Tags:         map[string]string{"foo": "bar"},
				Base:         params.Base{Name: "ubuntu", Channel: "18.04"},
				ImageRepo: params.DockerImageInfo{
					Repository:   "jujuqa",
					RegistryPath: "juju-operator-image",
				},
				CharmModifiedVersion: 1,
				Trust:                true,
				Scale:                3,
			}}}
		return nil
	})
	info, err := client.ProvisioningInfo(c.Context(), "gitlab")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.DeepEquals, caasapplicationprovisioner.ProvisioningInfo{
		Version:      vers,
		APIAddresses: []string{"10.0.0.1:1"},
		Tags:         map[string]string{"foo": "bar"},
		Base:         corebase.MakeDefaultBase("ubuntu", "18.04"),
		ImageDetails: params.ConvertDockerImageInfo(params.DockerImageInfo{
			Repository:   "jujuqa",
			RegistryPath: "juju-operator-image",
		}),
		CharmModifiedVersion: 1,
		Trust:                true,
		Scale:                3,
	})
}

func (s *provisionerSuite) TestProvisioningInfoArity(c *tc.C) {
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		c.Check(objType, tc.Equals, "CAASApplicationProvisioner")
		c.Check(id, tc.Equals, "")
		c.Assert(request, tc.Equals, "ProvisioningInfo")
		c.Assert(a, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "application-gitlab"}}})
		c.Assert(result, tc.FitsTypeOf, &params.CAASApplicationProvisioningInfoResults{})
		*(result.(*params.CAASApplicationProvisioningInfoResults)) = params.CAASApplicationProvisioningInfoResults{
			Results: []params.CAASApplicationProvisioningInfo{{}, {}},
		}
		return nil
	})
	_, err := client.ProvisioningInfo(c.Context(), "gitlab")
	c.Assert(err, tc.ErrorMatches, "expected one result, got 2")
}

func (s *provisionerSuite) TestRemoveUnit(c *tc.C) {
	var called bool
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		called = true
		c.Check(objType, tc.Equals, "CAASApplicationProvisioner")
		c.Check(id, tc.Equals, "")
		c.Assert(request, tc.Equals, "Remove")
		c.Assert(a, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "unit-foo-0"}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{}},
		}
		return nil
	})
	err := client.RemoveUnit(c.Context(), "foo/0")
	c.Check(err, tc.ErrorIsNil)
	c.Check(called, tc.IsTrue)
}

func (s *provisionerSuite) TestDestroyUnits(c *tc.C) {
	var called bool
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		called = true
		c.Check(objType, tc.Equals, "CAASApplicationProvisioner")
		c.Check(id, tc.Equals, "")
		c.Assert(request, tc.Equals, "DestroyUnits")
		c.Assert(a, tc.DeepEquals, params.DestroyUnitsParams{
			Units: []params.DestroyUnitParams{
				{
					UnitTag: "unit-foo-0",
				},
			},
		})
		c.Assert(result, tc.FitsTypeOf, &params.DestroyUnitResults{})
		*(result.(*params.DestroyUnitResults)) = params.DestroyUnitResults{
			Results: []params.DestroyUnitResult{
				{
					Info: &params.DestroyUnitInfo{},
				},
			},
		}
		return nil
	})
	err := client.DestroyUnits(c.Context(), []string{"foo/0"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(called, tc.IsTrue)
}

func (s *provisionerSuite) TestDestroyUnitsMismatchResults(c *tc.C) {
	var called bool
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		called = true
		c.Check(objType, tc.Equals, "CAASApplicationProvisioner")
		c.Check(id, tc.Equals, "")
		c.Assert(request, tc.Equals, "DestroyUnits")
		c.Assert(a, tc.DeepEquals, params.DestroyUnitsParams{
			Units: []params.DestroyUnitParams{
				{
					UnitTag: "unit-foo-0",
				},
			},
		})
		c.Assert(result, tc.FitsTypeOf, &params.DestroyUnitResults{})
		*(result.(*params.DestroyUnitResults)) = params.DestroyUnitResults{
			Results: []params.DestroyUnitResult{
				{
					Info: &params.DestroyUnitInfo{},
				},
				{
					Info: &params.DestroyUnitInfo{},
				},
			},
		}
		return nil
	})
	err := client.DestroyUnits(c.Context(), []string{"foo/0"})
	c.Assert(err, tc.NotNil)
	c.Assert(err.Error(), tc.Equals, "expected 1 results got 2")
	c.Assert(called, tc.IsTrue)
}
