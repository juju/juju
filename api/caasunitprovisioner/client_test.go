// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/caasunitprovisioner"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/life"
)

type unitprovisionerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&unitprovisionerSuite{})

func newClient(f basetesting.APICallerFunc) *caasunitprovisioner.Client {
	return caasunitprovisioner.NewClient(basetesting.BestVersionCaller{f, 1})
}

func (s *unitprovisionerSuite) TestPodSpec(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASUnitProvisioner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "PodSpec")
		c.Check(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{
				Tag: "application-gitlab",
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.StringResults{})
		*(result.(*params.StringResults)) = params.StringResults{
			Results: []params.StringResult{{
				Result: "foo",
			}},
		}
		return nil
	})

	client := caasunitprovisioner.NewClient(apiCaller)
	spec, err := client.PodSpec("gitlab")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(spec, gc.Equals, "foo")
}

func (s *unitprovisionerSuite) TestPodSpecError(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.StringResults)) = params.StringResults{
			Results: []params.StringResult{{Error: &params.Error{
				Code:    params.CodeNotFound,
				Message: "bletch",
			}}},
		}
		return nil
	})

	client := caasunitprovisioner.NewClient(apiCaller)
	_, err := client.PodSpec("gitlab")
	c.Assert(err, gc.ErrorMatches, "bletch")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *unitprovisionerSuite) TestPodSpecInvalidApplicationName(c *gc.C) {
	client := caasunitprovisioner.NewClient(basetesting.APICallerFunc(func(_ string, _ int, _, _ string, _, _ interface{}) error {
		return errors.New("should not be called")
	}))
	_, err := client.PodSpec("gitlab/0")
	c.Assert(err, gc.ErrorMatches, `application name "gitlab/0" not valid`)
}

func (s *unitprovisionerSuite) TestLife(c *gc.C) {
	s.testLife(c, names.NewApplicationTag("gitlab"))
	s.testLife(c, names.NewUnitTag("gitlab/0"))
}

func (s *unitprovisionerSuite) testLife(c *gc.C, tag names.Tag) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASUnitProvisioner")
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
				Life: params.Alive,
			}},
		}
		return nil
	})

	client := caasunitprovisioner.NewClient(apiCaller)
	lifeValue, err := client.Life(tag.Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(lifeValue, gc.Equals, life.Alive)
}

func (s *unitprovisionerSuite) TestLifeError(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.LifeResults)) = params.LifeResults{
			Results: []params.LifeResult{{Error: &params.Error{
				Code:    params.CodeNotFound,
				Message: "bletch",
			}}},
		}
		return nil
	})

	client := caasunitprovisioner.NewClient(apiCaller)
	_, err := client.Life("gitlab/0")
	c.Assert(err, gc.ErrorMatches, "bletch")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *unitprovisionerSuite) TestLifeInvalidEntityame(c *gc.C) {
	client := caasunitprovisioner.NewClient(basetesting.APICallerFunc(func(_ string, _ int, _, _ string, _, _ interface{}) error {
		return errors.New("should not be called")
	}))
	_, err := client.Life("")
	c.Assert(err, gc.ErrorMatches, `application or unit name "" not valid`)
}

func (s *unitprovisionerSuite) TestWatchApplications(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASUnitProvisioner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchApplications")
		c.Assert(result, gc.FitsTypeOf, &params.StringsWatchResult{})
		*(result.(*params.StringsWatchResult)) = params.StringsWatchResult{
			Error: &params.Error{Message: "FAIL"},
		}
		return nil
	})

	client := caasunitprovisioner.NewClient(apiCaller)
	watcher, err := client.WatchApplications()
	c.Assert(watcher, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "FAIL")
}

func (s *unitprovisionerSuite) TestWatchUnits(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASUnitProvisioner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchUnits")
		c.Assert(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{
				Tag: "application-gitlab",
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.StringsWatchResults{})
		*(result.(*params.StringsWatchResults)) = params.StringsWatchResults{
			Results: []params.StringsWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		return nil
	})

	client := caasunitprovisioner.NewClient(apiCaller)
	watcher, err := client.WatchUnits("gitlab")
	c.Assert(watcher, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "FAIL")
}

func (s *unitprovisionerSuite) TestWatchPodSpec(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASUnitProvisioner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchPodSpec")
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

	client := caasunitprovisioner.NewClient(apiCaller)
	watcher, err := client.WatchPodSpec("gitlab")
	c.Assert(watcher, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "FAIL")
}

func (s *unitprovisionerSuite) TestApplicationConfig(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASUnitProvisioner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "ApplicationsConfig")
		c.Assert(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{
				Tag: "application-gitlab",
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ApplicationGetConfigResults{})
		*(result.(*params.ApplicationGetConfigResults)) = params.ApplicationGetConfigResults{
			Results: []params.ConfigResult{{
				Config: map[string]interface{}{"foo": "bar"},
			}},
		}
		return nil
	})

	client := caasunitprovisioner.NewClient(apiCaller)
	cfg, err := client.ApplicationConfig("gitlab")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg, jc.DeepEquals, application.ConfigAttributes{"foo": "bar"})
}

func (s *unitprovisionerSuite) TestUpdateUnits(c *gc.C) {
	var called bool
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		called = true
		c.Check(objType, gc.Equals, "CAASUnitProvisioner")
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
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{}},
		}
		return nil
	})
	err := client.UpdateUnits(params.UpdateApplicationUnits{
		ApplicationTag: names.NewApplicationTag("app").String(),
		Units: []params.ApplicationUnitParams{
			{ProviderId: "uuid", UnitTag: "unit-gitlab-0", Address: "address", Ports: []string{"port"},
				Status: "active", Info: "message"},
		},
	})
	c.Check(err, jc.ErrorIsNil)
	c.Check(called, jc.IsTrue)
}

func (s *unitprovisionerSuite) TestUpdateUnitsCount(c *gc.C) {
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{
				{Error: &params.Error{Message: "FAIL"}},
				{Error: &params.Error{Message: "FAIL"}},
			},
		}
		return nil
	})
	err := client.UpdateUnits(params.UpdateApplicationUnits{
		ApplicationTag: names.NewApplicationTag("app").String(),
		Units: []params.ApplicationUnitParams{
			{ProviderId: "uuid", Address: "address"},
		},
	})
	c.Check(err, gc.ErrorMatches, `expected 1 result\(s\), got 2`)
}

func (s *unitprovisionerSuite) TestUpdateApplicationService(c *gc.C) {
	var called bool
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		called = true
		c.Check(objType, gc.Equals, "CAASUnitProvisioner")
		c.Check(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "UpdateApplicationsService")
		c.Assert(a, jc.DeepEquals, params.UpdateApplicationServiceArgs{
			Args: []params.UpdateApplicationServiceArg{
				{
					ApplicationTag: "application-app",
					ProviderId:     "id",
					Addresses:      []params.Address{{Value: "10.0.0.1"}},
				},
			},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{}},
		}
		return nil
	})
	err := client.UpdateApplicationService(params.UpdateApplicationServiceArg{
		ApplicationTag: names.NewApplicationTag("app").String(),
		ProviderId:     "id",
		Addresses:      []params.Address{{Value: "10.0.0.1"}},
	})
	c.Check(err, jc.ErrorIsNil)
	c.Check(called, jc.IsTrue)
}

func (s *unitprovisionerSuite) TestUpdateApplicationServiceCount(c *gc.C) {
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{
				{Error: &params.Error{Message: "FAIL"}},
				{Error: &params.Error{Message: "FAIL"}},
			},
		}
		return nil
	})
	err := client.UpdateApplicationService(params.UpdateApplicationServiceArg{
		ApplicationTag: names.NewApplicationTag("app").String(),
		ProviderId:     "id",
		Addresses:      []params.Address{{Value: "10.0.0.1"}},
	})
	c.Check(err, gc.ErrorMatches, `expected 1 result\(s\), got 2`)
}
