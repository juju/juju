// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner_test

import (
	"github.com/juju/charm/v8"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/caasapplicationprovisioner"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/resources"
	"github.com/juju/juju/core/status"
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

func (s *provisionerSuite) TestLife(c *gc.C) {
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
	c.Assert(err, gc.ErrorMatches, `application name "" not valid`)
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
				ImagePath:            "juju-operator-image",
				Version:              vers,
				APIAddresses:         []string{"10.0.0.1:1"},
				Tags:                 map[string]string{"foo": "bar"},
				Series:               "bionic",
				ImageRepo:            "jujuqa",
				CharmModifiedVersion: 1,
				CharmURL:             "cs:~test/charm-1",
			}}}
		return nil
	})
	info, err := client.ProvisioningInfo("gitlab")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, caasapplicationprovisioner.ProvisioningInfo{
		ImagePath:            "juju-operator-image",
		Version:              vers,
		APIAddresses:         []string{"10.0.0.1:1"},
		Tags:                 map[string]string{"foo": "bar"},
		Series:               "bionic",
		ImageRepo:            "jujuqa",
		CharmModifiedVersion: 1,
		CharmURL:             &charm.URL{Schema: "cs", User: "test", Name: "charm", Revision: 1},
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
			Username:     "jujuqa",
			Password:     "pwd",
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
		c.Assert(result, gc.FitsTypeOf, &params.EntitiesResults{})
		*(result.(*params.EntitiesResults)) = params.EntitiesResults{
			Results: []params.EntitiesResult{{
				Entities: []params.Entity{{"unit-gitlab-0"}},
			}},
		}
		return nil
	})

	tags, err := client.Units("gitlab")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tags, jc.SameContents, []names.Tag{names.NewUnitTag("gitlab/0")})
}

func (s *provisionerSuite) TestGarbageCollect(c *gc.C) {
	client := newClient(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASApplicationProvisioner")
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "CAASApplicationGarbageCollect")
		c.Assert(arg, jc.DeepEquals, params.CAASApplicationGarbageCollectArgs{Args: []params.CAASApplicationGarbageCollectArg{{
			Application:     params.Entity{Tag: "application-gitlab"},
			ObservedUnits:   params.Entities{[]params.Entity{{Tag: "unit-gitlab-0"}}},
			DesiredReplicas: 1,
			ActivePodNames:  []string{"gitlab-0"},
			Force:           false,
		}}})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: nil,
			}},
		}
		return nil
	})

	err := client.GarbageCollect("gitlab", []names.Tag{names.NewUnitTag("gitlab/0")}, 1, []string{"gitlab-0"}, false)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *provisionerSuite) TestApplicationCharmURL(c *gc.C) {
	client := newClient(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASApplicationProvisioner")
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "ApplicationCharmURLs")
		c.Assert(arg, jc.DeepEquals, params.Entities{Entities: []params.Entity{{"application-gitlab"}}})
		c.Assert(result, gc.FitsTypeOf, &params.StringResults{})
		*(result.(*params.StringResults)) = params.StringResults{
			Results: []params.StringResult{{
				Result: "cs:charm",
			}},
		}
		return nil
	})

	charmURL, err := client.ApplicationCharmURL("gitlab")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(charmURL, jc.DeepEquals, &charm.URL{Schema: "cs", Name: "charm", Revision: -1})
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
