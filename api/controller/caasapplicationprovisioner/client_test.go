// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner_test

import (
	"context"

	"github.com/juju/names/v6"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/caasapplicationprovisioner"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/resource"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/rpc/params"
)

type provisionerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&provisionerSuite{})

func newClient(f basetesting.APICallerFunc) *caasapplicationprovisioner.Client {
	return caasapplicationprovisioner.NewClient(basetesting.BestVersionCaller{APICallerFunc: f, BestVersion: 1})
}

func (s *provisionerSuite) TestSetPasswords(c *gc.C) {
	var called bool
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		called = true
		c.Check(objType, gc.Equals, "CAASApplicationProvisioner")
		c.Check(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "SetPasswords")
		c.Assert(a, jc.DeepEquals, params.EntityPasswords{
			Changes: []params.EntityPassword{{Tag: "application-app", Password: "secret"}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{}},
		}
		return nil
	})
	err := client.SetPassword(context.Background(), "app", "secret")
	c.Check(err, jc.ErrorIsNil)
	c.Check(called, jc.IsTrue)
}

func (s *provisionerSuite) TestProvisioningInfo(c *gc.C) {
	vers := semversion.MustParse("2.99.0")
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASApplicationProvisioner")
		c.Check(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "ProvisioningInfo")
		c.Assert(a, jc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "application-gitlab"}}})
		c.Assert(result, gc.FitsTypeOf, &params.CAASApplicationProvisioningInfoResults{})
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
				CharmURL:             "ch:charm-1",
				Trust:                true,
				Scale:                3,
			}}}
		return nil
	})
	info, err := client.ProvisioningInfo(context.Background(), "gitlab")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, caasapplicationprovisioner.ProvisioningInfo{
		Version:      vers,
		APIAddresses: []string{"10.0.0.1:1"},
		Tags:         map[string]string{"foo": "bar"},
		Base:         corebase.MakeDefaultBase("ubuntu", "18.04"),
		ImageDetails: params.ConvertDockerImageInfo(params.DockerImageInfo{
			Repository:   "jujuqa",
			RegistryPath: "juju-operator-image",
		}),
		CharmModifiedVersion: 1,
		CharmURL:             &charm.URL{Schema: "ch", Name: "charm", Revision: 1},
		Trust:                true,
		Scale:                3,
	})
}

func (s *provisionerSuite) TestApplicationOCIResources(c *gc.C) {
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASApplicationProvisioner")
		c.Check(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "ApplicationOCIResources")
		c.Assert(a, jc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "application-gitlab"}}})
		c.Assert(result, gc.FitsTypeOf, &params.CAASApplicationOCIResourceResults{})
		*(result.(*params.CAASApplicationOCIResourceResults)) = params.CAASApplicationOCIResourceResults{
			Results: []params.CAASApplicationOCIResourceResult{
				{
					Result: &params.CAASApplicationOCIResources{
						Images: map[string]params.DockerImageInfo{
							"cockroachdb-image": {
								RegistryPath: "cockroachdb/cockroach:v20.1.4",
								Username:     "jujuqa",
								Password:     "pwd",
							},
						},
					},
				},
			}}
		return nil
	})
	imageResources, err := client.ApplicationOCIResources(context.Background(), "gitlab")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(imageResources, jc.DeepEquals, map[string]resource.DockerImageDetails{
		"cockroachdb-image": {
			RegistryPath: "cockroachdb/cockroach:v20.1.4",
			ImageRepoDetails: resource.ImageRepoDetails{
				BasicAuthConfig: resource.BasicAuthConfig{
					Username: "jujuqa",
					Password: "pwd",
				},
			},
		},
	})
}

func (s *provisionerSuite) TestProvisioningInfoArity(c *gc.C) {
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASApplicationProvisioner")
		c.Check(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "ProvisioningInfo")
		c.Assert(a, jc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "application-gitlab"}}})
		c.Assert(result, gc.FitsTypeOf, &params.CAASApplicationProvisioningInfoResults{})
		*(result.(*params.CAASApplicationProvisioningInfoResults)) = params.CAASApplicationProvisioningInfoResults{
			Results: []params.CAASApplicationProvisioningInfo{{}, {}},
		}
		return nil
	})
	_, err := client.ProvisioningInfo(context.Background(), "gitlab")
	c.Assert(err, gc.ErrorMatches, "expected one result, got 2")
}

func (s *provisionerSuite) TestUpdateUnits(c *gc.C) {
	var called bool
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		called = true
		c.Check(objType, gc.Equals, "CAASApplicationProvisioner")
		c.Check(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "UpdateApplicationsUnits")
		c.Assert(a, jc.DeepEquals, params.UpdateApplicationUnitArgs{
			Args: []params.UpdateApplicationUnits{
				{
					ApplicationTag: "application-app",
					Units: []params.ApplicationUnitParams{
						{ProviderId: "uuid", UnitTag: "unit-gitlab-0", Address: "address", Ports: []string{"port"},
							Status: "active", Info: "message"},
					},
				},
			},
		})
		c.Assert(result, gc.FitsTypeOf, &params.UpdateApplicationUnitResults{})
		*(result.(*params.UpdateApplicationUnitResults)) = params.UpdateApplicationUnitResults{
			Results: []params.UpdateApplicationUnitResult{{
				Info: &params.UpdateApplicationUnitsInfo{
					Units: []params.ApplicationUnitInfo{
						{ProviderId: "uuid", UnitTag: "unit-gitlab-0"},
					},
				},
			}},
		}
		return nil
	})
	info, err := client.UpdateUnits(context.Background(), params.UpdateApplicationUnits{
		ApplicationTag: names.NewApplicationTag("app").String(),
		Units: []params.ApplicationUnitParams{
			{ProviderId: "uuid", UnitTag: "unit-gitlab-0", Address: "address", Ports: []string{"port"},
				Status: "active", Info: "message"},
		},
	})
	c.Check(err, jc.ErrorIsNil)
	c.Check(called, jc.IsTrue)
	c.Check(info, jc.DeepEquals, &params.UpdateApplicationUnitsInfo{
		Units: []params.ApplicationUnitInfo{
			{ProviderId: "uuid", UnitTag: "unit-gitlab-0"},
		},
	})
}

func (s *provisionerSuite) TestUpdateUnitsCount(c *gc.C) {
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASApplicationProvisioner")
		c.Assert(result, gc.FitsTypeOf, &params.UpdateApplicationUnitResults{})
		*(result.(*params.UpdateApplicationUnitResults)) = params.UpdateApplicationUnitResults{
			Results: []params.UpdateApplicationUnitResult{
				{Error: &params.Error{Message: "FAIL"}},
				{Error: &params.Error{Message: "FAIL"}},
			},
		}
		return nil
	})
	info, err := client.UpdateUnits(context.Background(), params.UpdateApplicationUnits{
		ApplicationTag: names.NewApplicationTag("app").String(),
		Units: []params.ApplicationUnitParams{
			{ProviderId: "uuid", Address: "address"},
		},
	})
	c.Check(err, gc.ErrorMatches, `expected 1 result\(s\), got 2`)
	c.Assert(info, gc.IsNil)
}

func (s *provisionerSuite) TestWatchApplication(c *gc.C) {
	client := newClient(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASApplicationProvisioner")
		c.Check(version, gc.Equals, 1)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "Watch")
		c.Assert(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{
				Tag: "application-gitlab",
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.NotifyWatchResults{})
		*(result.(*params.NotifyWatchResults)) = params.NotifyWatchResults{
			Results: []params.NotifyWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		return nil
	})
	watcher, err := client.WatchApplication(context.Background(), "gitlab")
	c.Assert(watcher, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "FAIL")
}

func (s *provisionerSuite) TestClearApplicationResources(c *gc.C) {
	var called bool
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		called = true
		c.Check(objType, gc.Equals, "CAASApplicationProvisioner")
		c.Check(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "ClearApplicationsResources")
		c.Assert(a, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "application-foo"}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{}},
		}
		return nil
	})
	err := client.ClearApplicationResources(context.Background(), "foo")
	c.Check(err, jc.ErrorIsNil)
	c.Check(called, jc.IsTrue)
}

func (s *provisionerSuite) TestWatchUnits(c *gc.C) {
	var called bool
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		called = true
		c.Check(objType, gc.Equals, "CAASApplicationProvisioner")
		c.Check(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "WatchUnits")
		c.Assert(a, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "application-foo"}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.StringsWatchResults{})
		*(result.(*params.StringsWatchResults)) = params.StringsWatchResults{
			Results: []params.StringsWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		return nil
	})
	worker, err := client.WatchUnits(context.Background(), "foo")
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(worker, gc.IsNil)
	c.Check(called, jc.IsTrue)
}

func (s *provisionerSuite) TestRemoveUnit(c *gc.C) {
	var called bool
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		called = true
		c.Check(objType, gc.Equals, "CAASApplicationProvisioner")
		c.Check(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "Remove")
		c.Assert(a, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "unit-foo-0"}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{}},
		}
		return nil
	})
	err := client.RemoveUnit(context.Background(), "foo/0")
	c.Check(err, jc.ErrorIsNil)
	c.Check(called, jc.IsTrue)
}

func (s *provisionerSuite) TestDestroyUnits(c *gc.C) {
	var called bool
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		called = true
		c.Check(objType, gc.Equals, "CAASApplicationProvisioner")
		c.Check(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "DestroyUnits")
		c.Assert(a, jc.DeepEquals, params.DestroyUnitsParams{
			Units: []params.DestroyUnitParams{
				{
					UnitTag: "unit-foo-0",
				},
			},
		})
		c.Assert(result, gc.FitsTypeOf, &params.DestroyUnitResults{})
		*(result.(*params.DestroyUnitResults)) = params.DestroyUnitResults{
			Results: []params.DestroyUnitResult{
				{
					Info: &params.DestroyUnitInfo{},
				},
			},
		}
		return nil
	})
	err := client.DestroyUnits(context.Background(), []string{"foo/0"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *provisionerSuite) TestDestroyUnitsMismatchResults(c *gc.C) {
	var called bool
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		called = true
		c.Check(objType, gc.Equals, "CAASApplicationProvisioner")
		c.Check(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "DestroyUnits")
		c.Assert(a, jc.DeepEquals, params.DestroyUnitsParams{
			Units: []params.DestroyUnitParams{
				{
					UnitTag: "unit-foo-0",
				},
			},
		})
		c.Assert(result, gc.FitsTypeOf, &params.DestroyUnitResults{})
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
	err := client.DestroyUnits(context.Background(), []string{"foo/0"})
	c.Assert(err, gc.NotNil)
	c.Assert(err.Error(), gc.Equals, "expected 1 results got 2")
	c.Assert(called, jc.IsTrue)
}
