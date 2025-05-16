// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/caasapplicationprovisioner"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/resource"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

type provisionerSuite struct {
	testhelpers.IsolationSuite
}

func TestProvisionerSuite(t *stdtesting.T) { tc.Run(t, &provisionerSuite{}) }
func newClient(f basetesting.APICallerFunc) *caasapplicationprovisioner.Client {
	return caasapplicationprovisioner.NewClient(basetesting.BestVersionCaller{APICallerFunc: f, BestVersion: 1})
}

func (s *provisionerSuite) TestWatchApplications(c *tc.C) {
	var called bool
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		called = true
		c.Check(objType, tc.Equals, "CAASApplicationProvisioner")
		c.Check(id, tc.Equals, "")
		c.Assert(request, tc.Equals, "WatchApplications")
		c.Assert(a, tc.IsNil)
		c.Assert(result, tc.FitsTypeOf, &params.StringsWatchResult{})
		*(result.(*params.StringsWatchResult)) = params.StringsWatchResult{
			Error: &params.Error{Message: "FAIL"},
		}
		return nil
	})
	_, err := client.WatchApplications(c.Context())
	c.Check(err, tc.ErrorMatches, "FAIL")
	c.Check(called, tc.IsTrue)
}

func (s *provisionerSuite) TestSetPasswords(c *tc.C) {
	var called bool
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		called = true
		c.Check(objType, tc.Equals, "CAASApplicationProvisioner")
		c.Check(id, tc.Equals, "")
		c.Assert(request, tc.Equals, "SetPasswords")
		c.Assert(a, tc.DeepEquals, params.EntityPasswords{
			Changes: []params.EntityPassword{{Tag: "application-app", Password: "secret"}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{}},
		}
		return nil
	})
	err := client.SetPassword(c.Context(), "app", "secret")
	c.Check(err, tc.ErrorIsNil)
	c.Check(called, tc.IsTrue)
}

func (s *provisionerSuite) TestLifeApplication(c *tc.C) {
	tag := names.NewApplicationTag("app")
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "CAASApplicationProvisioner")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "Life")
		c.Check(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{
				Tag: tag.String(),
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.LifeResults{})
		*(result.(*params.LifeResults)) = params.LifeResults{
			Results: []params.LifeResult{{
				Life: life.Alive,
			}},
		}
		return nil
	})

	client := caasapplicationprovisioner.NewClient(apiCaller)
	lifeValue, err := client.Life(c.Context(), tag.Id())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(lifeValue, tc.Equals, life.Alive)
}

func (s *provisionerSuite) TestLifeUnit(c *tc.C) {
	tag := names.NewUnitTag("foo/0")
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "CAASApplicationProvisioner")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "Life")
		c.Check(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{
				Tag: "unit-foo-0",
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.LifeResults{})
		*(result.(*params.LifeResults)) = params.LifeResults{
			Results: []params.LifeResult{{
				Life: life.Alive,
			}},
		}
		return nil
	})

	client := caasapplicationprovisioner.NewClient(apiCaller)
	lifeValue, err := client.Life(c.Context(), tag.Id())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(lifeValue, tc.Equals, life.Alive)
}

func (s *provisionerSuite) TestLifeError(c *tc.C) {
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
	_, err := client.Life(c.Context(), "gitlab")
	c.Assert(err, tc.ErrorMatches, "bletch")
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}

func (s *provisionerSuite) TestLifeInvalidApplicationName(c *tc.C) {
	client := caasapplicationprovisioner.NewClient(basetesting.APICallerFunc(func(_ string, _ int, _, _ string, _, _ interface{}) error {
		return errors.New("should not be called")
	}))
	_, err := client.Life(c.Context(), "")
	c.Assert(err, tc.ErrorMatches, `application or unit name "" not valid`)
}

func (s *provisionerSuite) TestLifeCount(c *tc.C) {
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		*(result.(*params.LifeResults)) = params.LifeResults{
			Results: []params.LifeResult{
				{Error: &params.Error{Message: "FAIL"}},
				{Error: &params.Error{Message: "FAIL"}},
			},
		}
		return nil
	})
	_, err := client.Life(c.Context(), "gitlab")
	c.Check(err, tc.ErrorMatches, `expected 1 result, got 2`)
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
				CharmURL:             "ch:charm-1",
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
		CharmURL:             &charm.URL{Schema: "ch", Name: "charm", Revision: 1},
		Trust:                true,
		Scale:                3,
	})
}

func (s *provisionerSuite) TestApplicationOCIResources(c *tc.C) {
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		c.Check(objType, tc.Equals, "CAASApplicationProvisioner")
		c.Check(id, tc.Equals, "")
		c.Assert(request, tc.Equals, "ApplicationOCIResources")
		c.Assert(a, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "application-gitlab"}}})
		c.Assert(result, tc.FitsTypeOf, &params.CAASApplicationOCIResourceResults{})
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
	imageResources, err := client.ApplicationOCIResources(c.Context(), "gitlab")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(imageResources, tc.DeepEquals, map[string]resource.DockerImageDetails{
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

func (s *provisionerSuite) TestSetOperatorStatus(c *tc.C) {
	client := newClient(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "CAASApplicationProvisioner")
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "SetOperatorStatus")
		c.Assert(arg, tc.DeepEquals, params.SetStatus{
			Entities: []params.EntityStatusArgs{{
				Tag:    "application-gitlab",
				Status: "error",
				Info:   "broken",
				Data:   map[string]interface{}{"foo": "bar"},
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		return nil
	})

	err := client.SetOperatorStatus(c.Context(), "gitlab", status.Error, "broken", map[string]interface{}{"foo": "bar"})
	c.Assert(err, tc.ErrorMatches, "FAIL")
}

func (s *provisionerSuite) TestAllUnits(c *tc.C) {
	client := newClient(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "CAASApplicationProvisioner")
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "Units")
		c.Assert(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "application-gitlab"}}})
		c.Assert(result, tc.FitsTypeOf, &params.CAASUnitsResults{})
		*(result.(*params.CAASUnitsResults)) = params.CAASUnitsResults{
			Results: []params.CAASUnitsResult{{
				Units: []params.CAASUnitInfo{
					{Tag: "unit-gitlab-0"},
				},
			}},
		}
		return nil
	})

	tags, err := client.Units(c.Context(), "gitlab")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(tags, tc.SameContents, []params.CAASUnit{
		{Tag: names.NewUnitTag("gitlab/0")},
	})
}

func (s *provisionerSuite) TestUpdateUnits(c *tc.C) {
	var called bool
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		called = true
		c.Check(objType, tc.Equals, "CAASApplicationProvisioner")
		c.Check(id, tc.Equals, "")
		c.Assert(request, tc.Equals, "UpdateApplicationsUnits")
		c.Assert(a, tc.DeepEquals, params.UpdateApplicationUnitArgs{
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
		c.Assert(result, tc.FitsTypeOf, &params.UpdateApplicationUnitResults{})
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
	info, err := client.UpdateUnits(c.Context(), params.UpdateApplicationUnits{
		ApplicationTag: names.NewApplicationTag("app").String(),
		Units: []params.ApplicationUnitParams{
			{ProviderId: "uuid", UnitTag: "unit-gitlab-0", Address: "address", Ports: []string{"port"},
				Status: "active", Info: "message"},
		},
	})
	c.Check(err, tc.ErrorIsNil)
	c.Check(called, tc.IsTrue)
	c.Check(info, tc.DeepEquals, &params.UpdateApplicationUnitsInfo{
		Units: []params.ApplicationUnitInfo{
			{ProviderId: "uuid", UnitTag: "unit-gitlab-0"},
		},
	})
}

func (s *provisionerSuite) TestUpdateUnitsCount(c *tc.C) {
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		c.Check(objType, tc.Equals, "CAASApplicationProvisioner")
		c.Assert(result, tc.FitsTypeOf, &params.UpdateApplicationUnitResults{})
		*(result.(*params.UpdateApplicationUnitResults)) = params.UpdateApplicationUnitResults{
			Results: []params.UpdateApplicationUnitResult{
				{Error: &params.Error{Message: "FAIL"}},
				{Error: &params.Error{Message: "FAIL"}},
			},
		}
		return nil
	})
	info, err := client.UpdateUnits(c.Context(), params.UpdateApplicationUnits{
		ApplicationTag: names.NewApplicationTag("app").String(),
		Units: []params.ApplicationUnitParams{
			{ProviderId: "uuid", Address: "address"},
		},
	})
	c.Check(err, tc.ErrorMatches, `expected 1 result\(s\), got 2`)
	c.Assert(info, tc.IsNil)
}

func (s *provisionerSuite) TestWatchApplication(c *tc.C) {
	client := newClient(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "CAASApplicationProvisioner")
		c.Check(version, tc.Equals, 1)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "Watch")
		c.Assert(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{
				Tag: "application-gitlab",
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.NotifyWatchResults{})
		*(result.(*params.NotifyWatchResults)) = params.NotifyWatchResults{
			Results: []params.NotifyWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		return nil
	})
	watcher, err := client.WatchApplication(c.Context(), "gitlab")
	c.Assert(watcher, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "FAIL")
}

func (s *provisionerSuite) TestClearApplicationResources(c *tc.C) {
	var called bool
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		called = true
		c.Check(objType, tc.Equals, "CAASApplicationProvisioner")
		c.Check(id, tc.Equals, "")
		c.Assert(request, tc.Equals, "ClearApplicationsResources")
		c.Assert(a, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "application-foo"}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{}},
		}
		return nil
	})
	err := client.ClearApplicationResources(c.Context(), "foo")
	c.Check(err, tc.ErrorIsNil)
	c.Check(called, tc.IsTrue)
}

func (s *provisionerSuite) TestWatchUnits(c *tc.C) {
	var called bool
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		called = true
		c.Check(objType, tc.Equals, "CAASApplicationProvisioner")
		c.Check(id, tc.Equals, "")
		c.Assert(request, tc.Equals, "WatchUnits")
		c.Assert(a, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "application-foo"}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.StringsWatchResults{})
		*(result.(*params.StringsWatchResults)) = params.StringsWatchResults{
			Results: []params.StringsWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		return nil
	})
	worker, err := client.WatchUnits(c.Context(), "foo")
	c.Check(err, tc.ErrorMatches, "FAIL")
	c.Check(worker, tc.IsNil)
	c.Check(called, tc.IsTrue)
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

func (s *provisionerSuite) TestProvisioningState(c *tc.C) {
	var called bool
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		called = true
		c.Check(objType, tc.Equals, "CAASApplicationProvisioner")
		c.Check(id, tc.Equals, "")
		c.Assert(request, tc.Equals, "ProvisioningState")
		c.Assert(a, tc.DeepEquals, params.Entity{Tag: "application-foo"})
		c.Assert(result, tc.FitsTypeOf, &params.CAASApplicationProvisioningStateResult{})
		*(result.(*params.CAASApplicationProvisioningStateResult)) = params.CAASApplicationProvisioningStateResult{
			ProvisioningState: &params.CAASApplicationProvisioningState{
				Scaling:     true,
				ScaleTarget: 10,
			},
		}
		return nil
	})
	state, err := client.ProvisioningState(c.Context(), "foo")
	c.Check(err, tc.ErrorIsNil)
	c.Check(called, tc.IsTrue)
	c.Check(state, tc.DeepEquals, &params.CAASApplicationProvisioningState{
		Scaling:     true,
		ScaleTarget: 10,
	})
}

func (s *provisionerSuite) TestSetProvisioningState(c *tc.C) {
	var called bool
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		called = true
		c.Check(objType, tc.Equals, "CAASApplicationProvisioner")
		c.Check(id, tc.Equals, "")
		c.Assert(request, tc.Equals, "SetProvisioningState")
		c.Assert(a, tc.DeepEquals, params.CAASApplicationProvisioningStateArg{
			Application: params.Entity{Tag: "application-foo"},
			ProvisioningState: params.CAASApplicationProvisioningState{
				Scaling:     true,
				ScaleTarget: 10,
			},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResult{})
		*(result.(*params.ErrorResult)) = params.ErrorResult{}
		return nil
	})
	err := client.SetProvisioningState(c.Context(), "foo", params.CAASApplicationProvisioningState{
		Scaling:     true,
		ScaleTarget: 10,
	})
	c.Check(err, tc.ErrorIsNil)
	c.Check(called, tc.IsTrue)
}

func (s *provisionerSuite) TestSetProvisioningStateError(c *tc.C) {
	var called bool
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		called = true
		c.Check(objType, tc.Equals, "CAASApplicationProvisioner")
		c.Check(id, tc.Equals, "")
		c.Assert(request, tc.Equals, "SetProvisioningState")
		c.Assert(a, tc.DeepEquals, params.CAASApplicationProvisioningStateArg{
			Application: params.Entity{Tag: "application-foo"},
			ProvisioningState: params.CAASApplicationProvisioningState{
				Scaling:     true,
				ScaleTarget: 10,
			},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResult{})
		*(result.(*params.ErrorResult)) = params.ErrorResult{
			Error: &params.Error{Code: params.CodeTryAgain},
		}
		return nil
	})
	err := client.SetProvisioningState(c.Context(), "foo", params.CAASApplicationProvisioningState{
		Scaling:     true,
		ScaleTarget: 10,
	})
	c.Check(params.IsCodeTryAgain(err), tc.IsTrue)
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

func (s *provisionerSuite) TestProvisionerConfig(c *tc.C) {
	var called bool
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		called = true
		c.Check(objType, tc.Equals, "CAASApplicationProvisioner")
		c.Check(id, tc.Equals, "")
		c.Assert(request, tc.Equals, "ProvisionerConfig")
		c.Assert(a, tc.IsNil)
		c.Assert(result, tc.FitsTypeOf, &params.CAASApplicationProvisionerConfigResult{})
		*(result.(*params.CAASApplicationProvisionerConfigResult)) = params.CAASApplicationProvisionerConfigResult{
			ProvisionerConfig: &params.CAASApplicationProvisionerConfig{
				UnmanagedApplications: params.Entities{Entities: []params.Entity{{Tag: "application-controller"}}},
			},
		}
		return nil
	})
	result, err := client.ProvisionerConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(called, tc.IsTrue)
	c.Assert(result, tc.DeepEquals, params.CAASApplicationProvisionerConfig{
		UnmanagedApplications: params.Entities{Entities: []params.Entity{{Tag: "application-controller"}}},
	})
}
