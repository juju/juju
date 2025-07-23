// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner_test

import (
	"github.com/juju/charm/v12"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/caasapplicationprovisioner"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/resources"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/docker"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/storage"
)

type provisionerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&provisionerSuite{})

func newClient(f basetesting.APICallerFunc) *caasapplicationprovisioner.Client {
	return caasapplicationprovisioner.NewClient(basetesting.BestVersionCaller{f, 1})
}

func (s *provisionerSuite) TestWatchApplications(c *gc.C) {
	var called bool
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		called = true
		c.Check(objType, gc.Equals, "CAASApplicationProvisioner")
		c.Check(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "WatchApplications")
		c.Assert(a, gc.IsNil)
		c.Assert(result, gc.FitsTypeOf, &params.StringsWatchResult{})
		*(result.(*params.StringsWatchResult)) = params.StringsWatchResult{
			Error: &params.Error{Message: "FAIL"},
		}
		return nil
	})
	_, err := client.WatchApplications()
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(called, jc.IsTrue)
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
	err := client.SetPassword("app", "secret")
	c.Check(err, jc.ErrorIsNil)
	c.Check(called, jc.IsTrue)
}

func (s *provisionerSuite) TestLifeApplication(c *gc.C) {
	tag := names.NewApplicationTag("app")
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASApplicationProvisioner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "Life")
		c.Check(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{
				Tag: tag.String(),
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.LifeResults{})
		*(result.(*params.LifeResults)) = params.LifeResults{
			Results: []params.LifeResult{{
				Life: life.Alive,
			}},
		}
		return nil
	})

	client := caasapplicationprovisioner.NewClient(apiCaller)
	lifeValue, err := client.Life(tag.Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(lifeValue, gc.Equals, life.Alive)
}

func (s *provisionerSuite) TestLifeUnit(c *gc.C) {
	tag := names.NewUnitTag("foo/0")
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASApplicationProvisioner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "Life")
		c.Check(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{
				Tag: "unit-foo-0",
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.LifeResults{})
		*(result.(*params.LifeResults)) = params.LifeResults{
			Results: []params.LifeResult{{
				Life: life.Alive,
			}},
		}
		return nil
	})

	client := caasapplicationprovisioner.NewClient(apiCaller)
	lifeValue, err := client.Life(tag.Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(lifeValue, gc.Equals, life.Alive)
}

func (s *provisionerSuite) TestLifeError(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.LifeResults)) = params.LifeResults{
			Results: []params.LifeResult{{Error: &params.Error{
				Code:    params.CodeNotFound,
				Message: "bletch",
			}}},
		}
		return nil
	})

	client := caasapplicationprovisioner.NewClient(apiCaller)
	_, err := client.Life("gitlab")
	c.Assert(err, gc.ErrorMatches, "bletch")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *provisionerSuite) TestLifeInvalidApplicationName(c *gc.C) {
	client := caasapplicationprovisioner.NewClient(basetesting.APICallerFunc(func(_ string, _ int, _, _ string, _, _ interface{}) error {
		return errors.New("should not be called")
	}))
	_, err := client.Life("")
	c.Assert(err, gc.ErrorMatches, `application or unit name "" not valid`)
}

func (s *provisionerSuite) TestLifeCount(c *gc.C) {
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		*(result.(*params.LifeResults)) = params.LifeResults{
			Results: []params.LifeResult{
				{Error: &params.Error{Message: "FAIL"}},
				{Error: &params.Error{Message: "FAIL"}},
			},
		}
		return nil
	})
	_, err := client.Life("gitlab")
	c.Check(err, gc.ErrorMatches, `expected 1 result, got 2`)
}

func (s *provisionerSuite) TestProvisioningInfo(c *gc.C) {
	vers := version.MustParse("2.99.0")
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASApplicationProvisioner")
		c.Check(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "ProvisioningInfo")
		c.Assert(a, jc.DeepEquals, params.Entities{Entities: []params.Entity{{"application-gitlab"}}})
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
	info, err := client.ProvisioningInfo("gitlab")
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

func (s *provisionerSuite) TestProvisioningInfoAttachStorage(c *gc.C) {
	vers := version.MustParse("2.99.0")
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASApplicationProvisioner")
		c.Check(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "ProvisioningInfo")
		c.Assert(a, jc.DeepEquals, params.Entities{Entities: []params.Entity{{"application-gitlab"}}})
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
				Scale:                1,
				FilesystemUnitAttachments: map[string][]params.KubernetesFilesystemUnitAttachmentParams{
					"data": {
						{
							UnitTag: "unit-charm-0", VolumeId: "pvc-foo-0",
						},
						{
							UnitTag: "unit-charm-1", VolumeId: "pvc-foo-1",
						},
					},
					"config": {
						{
							UnitTag: "unit-charm-0", VolumeId: "pvc-bar-0",
						},
						{
							UnitTag: "unit-charm-1", VolumeId: "pvc-bar-1",
						},
					},
				},
			}}}
		return nil
	})
	info, err := client.ProvisioningInfo("gitlab")
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
		Scale:                1,
		FilesystemUnitAttachments: map[string][]storage.KubernetesFilesystemUnitAttachmentParams{
			"data": {
				{UnitName: "charm/0", VolumeId: "pvc-foo-0"},
				{UnitName: "charm/1", VolumeId: "pvc-foo-1"},
			},
			"config": {
				{UnitName: "charm/0", VolumeId: "pvc-bar-0"},
				{UnitName: "charm/1", VolumeId: "pvc-bar-1"},
			},
		},
	})
}

func (s *provisionerSuite) TestApplicationOCIResources(c *gc.C) {
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASApplicationProvisioner")
		c.Check(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "ApplicationOCIResources")
		c.Assert(a, jc.DeepEquals, params.Entities{Entities: []params.Entity{{"application-gitlab"}}})
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
	imageResources, err := client.ApplicationOCIResources("gitlab")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(imageResources, jc.DeepEquals, map[string]resources.DockerImageDetails{
		"cockroachdb-image": {
			RegistryPath: "cockroachdb/cockroach:v20.1.4",
			ImageRepoDetails: docker.ImageRepoDetails{
				BasicAuthConfig: docker.BasicAuthConfig{
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
		c.Assert(a, jc.DeepEquals, params.Entities{Entities: []params.Entity{{"application-gitlab"}}})
		c.Assert(result, gc.FitsTypeOf, &params.CAASApplicationProvisioningInfoResults{})
		*(result.(*params.CAASApplicationProvisioningInfoResults)) = params.CAASApplicationProvisioningInfoResults{
			Results: []params.CAASApplicationProvisioningInfo{{}, {}},
		}
		return nil
	})
	_, err := client.ProvisioningInfo("gitlab")
	c.Assert(err, gc.ErrorMatches, "expected one result, got 2")
}

func (s *provisionerSuite) TestSetOperatorStatus(c *gc.C) {
	client := newClient(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASApplicationProvisioner")
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "SetOperatorStatus")
		c.Assert(arg, jc.DeepEquals, params.SetStatus{
			Entities: []params.EntityStatusArgs{{
				Tag:    "application-gitlab",
				Status: "error",
				Info:   "broken",
				Data:   map[string]interface{}{"foo": "bar"},
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		return nil
	})

	err := client.SetOperatorStatus("gitlab", status.Error, "broken", map[string]interface{}{"foo": "bar"})
	c.Assert(err, gc.ErrorMatches, "FAIL")
}

func (s *provisionerSuite) TestAllUnits(c *gc.C) {
	client := newClient(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASApplicationProvisioner")
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "Units")
		c.Assert(arg, jc.DeepEquals, params.Entities{Entities: []params.Entity{{"application-gitlab"}}})
		c.Assert(result, gc.FitsTypeOf, &params.CAASUnitsResults{})
		*(result.(*params.CAASUnitsResults)) = params.CAASUnitsResults{
			Results: []params.CAASUnitsResult{{
				Units: []params.CAASUnitInfo{
					{Tag: "unit-gitlab-0"},
				},
			}},
		}
		return nil
	})

	tags, err := client.Units("gitlab")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tags, jc.SameContents, []params.CAASUnit{
		{Tag: names.NewUnitTag("gitlab/0")},
	})
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
	info, err := client.UpdateUnits(params.UpdateApplicationUnits{
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
	info, err := client.UpdateUnits(params.UpdateApplicationUnits{
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
	watcher, err := client.WatchApplication("gitlab")
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
	err := client.ClearApplicationResources("foo")
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
	worker, err := client.WatchUnits("foo")
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
	err := client.RemoveUnit("foo/0")
	c.Check(err, jc.ErrorIsNil)
	c.Check(called, jc.IsTrue)
}

func (s *provisionerSuite) TestProvisioningState(c *gc.C) {
	var called bool
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		called = true
		c.Check(objType, gc.Equals, "CAASApplicationProvisioner")
		c.Check(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "ProvisioningState")
		c.Assert(a, jc.DeepEquals, params.Entity{Tag: "application-foo"})
		c.Assert(result, gc.FitsTypeOf, &params.CAASApplicationProvisioningStateResult{})
		*(result.(*params.CAASApplicationProvisioningStateResult)) = params.CAASApplicationProvisioningStateResult{
			ProvisioningState: &params.CAASApplicationProvisioningState{
				Scaling:     true,
				ScaleTarget: 10,
			},
		}
		return nil
	})
	state, err := client.ProvisioningState("foo")
	c.Check(err, jc.ErrorIsNil)
	c.Check(called, jc.IsTrue)
	c.Check(state, jc.DeepEquals, &params.CAASApplicationProvisioningState{
		Scaling:     true,
		ScaleTarget: 10,
	})
}

func (s *provisionerSuite) TestSetProvisioningState(c *gc.C) {
	var called bool
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		called = true
		c.Check(objType, gc.Equals, "CAASApplicationProvisioner")
		c.Check(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "SetProvisioningState")
		c.Assert(a, jc.DeepEquals, params.CAASApplicationProvisioningStateArg{
			Application: params.Entity{Tag: "application-foo"},
			ProvisioningState: params.CAASApplicationProvisioningState{
				Scaling:     true,
				ScaleTarget: 10,
			},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResult{})
		*(result.(*params.ErrorResult)) = params.ErrorResult{}
		return nil
	})
	err := client.SetProvisioningState("foo", params.CAASApplicationProvisioningState{
		Scaling:     true,
		ScaleTarget: 10,
	})
	c.Check(err, jc.ErrorIsNil)
	c.Check(called, jc.IsTrue)
}

func (s *provisionerSuite) TestSetProvisioningStateError(c *gc.C) {
	var called bool
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		called = true
		c.Check(objType, gc.Equals, "CAASApplicationProvisioner")
		c.Check(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "SetProvisioningState")
		c.Assert(a, jc.DeepEquals, params.CAASApplicationProvisioningStateArg{
			Application: params.Entity{Tag: "application-foo"},
			ProvisioningState: params.CAASApplicationProvisioningState{
				Scaling:     true,
				ScaleTarget: 10,
			},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResult{})
		*(result.(*params.ErrorResult)) = params.ErrorResult{
			Error: &params.Error{Code: params.CodeTryAgain},
		}
		return nil
	})
	err := client.SetProvisioningState("foo", params.CAASApplicationProvisioningState{
		Scaling:     true,
		ScaleTarget: 10,
	})
	c.Check(params.IsCodeTryAgain(err), jc.IsTrue)
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
	err := client.DestroyUnits([]string{"foo/0"})
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
	err := client.DestroyUnits([]string{"foo/0"})
	c.Assert(err, gc.NotNil)
	c.Assert(err.Error(), gc.Equals, "expected 1 results got 2")
	c.Assert(called, jc.IsTrue)
}

func (s *provisionerSuite) TestProvisionerConfig(c *gc.C) {
	var called bool
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		called = true
		c.Check(objType, gc.Equals, "CAASApplicationProvisioner")
		c.Check(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "ProvisionerConfig")
		c.Assert(a, gc.IsNil)
		c.Assert(result, gc.FitsTypeOf, &params.CAASApplicationProvisionerConfigResult{})
		*(result.(*params.CAASApplicationProvisionerConfigResult)) = params.CAASApplicationProvisionerConfigResult{
			ProvisionerConfig: &params.CAASApplicationProvisionerConfig{
				UnmanagedApplications: params.Entities{Entities: []params.Entity{{Tag: "application-controller"}}},
			},
		}
		return nil
	})
	result, err := client.ProvisionerConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
	c.Assert(result, gc.DeepEquals, params.CAASApplicationProvisionerConfig{
		UnmanagedApplications: params.Entities{Entities: []params.Entity{{Tag: "application-controller"}}},
	})
}

func (s *provisionerSuite) TestFilesystemProvisioningInfo(c *gc.C) {
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASApplicationProvisioner")
		c.Check(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "FilesystemProvisioningInfo")
		c.Assert(a, jc.DeepEquals, params.Entity{Tag: "application-gitlab"})
		c.Assert(result, gc.FitsTypeOf, &params.CAASApplicationFilesystemProvisioningInfoResult{})
		*(result.(*params.CAASApplicationFilesystemProvisioningInfoResult)) = params.CAASApplicationFilesystemProvisioningInfoResult{
			Result: &params.CAASApplicationFilesystemProvisioningInfo{
				Filesystems: []params.KubernetesFilesystemParams{
					{
						StorageName: "data",
						Provider:    "kubernetes",
						Size:        1024,
						Attributes:  map[string]interface{}{"storage-class": "fast"},
						Tags:        map[string]string{"env": "prod"},
						Attachment: &params.KubernetesFilesystemAttachmentParams{
							Provider:   "kubernetes",
							MountPoint: "/data",
							ReadOnly:   false,
						},
					},
				},
				FilesystemUnitAttachments: map[string][]params.KubernetesFilesystemUnitAttachmentParams{
					"data": {
						{
							UnitTag:  "unit-gitlab-0",
							VolumeId: "pvc-data-0",
						},
						{
							UnitTag:  "unit-gitlab-1",
							VolumeId: "pvc-data-1",
						},
					},
				},
			},
		}
		return nil
	})
	info, err := client.FilesystemProvisioningInfo("gitlab")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, caasapplicationprovisioner.FilesystemProvisioningInfo{
		Filesystems: []storage.KubernetesFilesystemParams{
			{
				StorageName:  "data",
				Provider:     storage.ProviderType("kubernetes"),
				Size:         1024,
				Attributes:   map[string]interface{}{"storage-class": "fast"},
				ResourceTags: map[string]string{"env": "prod"},
				Attachment: &storage.KubernetesFilesystemAttachmentParams{
					AttachmentParams: storage.AttachmentParams{
						Provider: storage.ProviderType("kubernetes"),
						ReadOnly: false,
					},
					Path: "/data",
				},
			},
		},
		FilesystemUnitAttachments: map[string][]storage.KubernetesFilesystemUnitAttachmentParams{
			"data": {
				{UnitName: "gitlab/0", VolumeId: "pvc-data-0"},
				{UnitName: "gitlab/1", VolumeId: "pvc-data-1"},
			},
		},
	})
}

func (s *provisionerSuite) TestFilesystemProvisioningInfoEmpty(c *gc.C) {
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASApplicationProvisioner")
		c.Check(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "FilesystemProvisioningInfo")
		c.Assert(a, jc.DeepEquals, params.Entity{Tag: "application-gitlab"})
		c.Assert(result, gc.FitsTypeOf, &params.CAASApplicationFilesystemProvisioningInfoResult{})
		*(result.(*params.CAASApplicationFilesystemProvisioningInfoResult)) = params.CAASApplicationFilesystemProvisioningInfoResult{
			Result: &params.CAASApplicationFilesystemProvisioningInfo{},
		}
		return nil
	})
	info, err := client.FilesystemProvisioningInfo("gitlab")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, caasapplicationprovisioner.FilesystemProvisioningInfo{})
}

func (s *provisionerSuite) TestFilesystemProvisioningInfoWithoutAttachment(c *gc.C) {
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASApplicationProvisioner")
		c.Check(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "FilesystemProvisioningInfo")
		c.Assert(a, jc.DeepEquals, params.Entity{Tag: "application-gitlab"})
		c.Assert(result, gc.FitsTypeOf, &params.CAASApplicationFilesystemProvisioningInfoResult{})
		*(result.(*params.CAASApplicationFilesystemProvisioningInfoResult)) = params.CAASApplicationFilesystemProvisioningInfoResult{
			Result: &params.CAASApplicationFilesystemProvisioningInfo{
				Filesystems: []params.KubernetesFilesystemParams{
					{
						StorageName: "logs",
						Provider:    "local",
						Size:        512,
					},
				},
			},
		}
		return nil
	})
	info, err := client.FilesystemProvisioningInfo("gitlab")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, caasapplicationprovisioner.FilesystemProvisioningInfo{
		Filesystems: []storage.KubernetesFilesystemParams{
			{
				StorageName: "logs",
				Provider:    storage.ProviderType("local"),
				Size:        512,
			},
		},
	})
}

func (s *provisionerSuite) TestFilesystemProvisioningInfoInvalidUnitTag(c *gc.C) {
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASApplicationProvisioner")
		c.Check(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "FilesystemProvisioningInfo")
		c.Assert(a, jc.DeepEquals, params.Entity{Tag: "application-gitlab"})
		c.Assert(result, gc.FitsTypeOf, &params.CAASApplicationFilesystemProvisioningInfoResult{})
		*(result.(*params.CAASApplicationFilesystemProvisioningInfoResult)) = params.CAASApplicationFilesystemProvisioningInfoResult{
			Result: &params.CAASApplicationFilesystemProvisioningInfo{
				FilesystemUnitAttachments: map[string][]params.KubernetesFilesystemUnitAttachmentParams{
					"data": {
						{
							UnitTag:  "invalid-tag",
							VolumeId: "pvc-data-0",
						},
					},
				},
			},
		}
		return nil
	})
	_, err := client.FilesystemProvisioningInfo("gitlab")
	c.Assert(err, gc.ErrorMatches, `"invalid-tag" is not a valid tag`)
}
