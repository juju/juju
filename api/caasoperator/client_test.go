// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/caasoperator"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
)

type operatorSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&operatorSuite{})

func (s *operatorSuite) TestSetStatus(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASOperator")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "SetStatus")
		c.Check(arg, jc.DeepEquals, params.SetStatus{
			Entities: []params.EntityStatusArgs{{
				Tag:    "application-gitlab",
				Status: "foo",
				Info:   "bar",
				Data: map[string]interface{}{
					"baz": "qux",
				},
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: &params.Error{Message: "bletch"}}},
		}
		return nil
	})

	client := caasoperator.NewClient(apiCaller)
	err := client.SetStatus("gitlab", "foo", "bar", map[string]interface{}{
		"baz": "qux",
	})
	c.Assert(err, gc.ErrorMatches, "bletch")
}

func (s *operatorSuite) TestSetStatusInvalidApplicationName(c *gc.C) {
	client := caasoperator.NewClient(basetesting.APICallerFunc(func(_ string, _ int, _, _ string, _, _ interface{}) error {
		return errors.New("should not be called")
	}))
	err := client.SetStatus("", "foo", "bar", nil)
	c.Assert(err, gc.ErrorMatches, `application name "" not valid`)
}

func (s *operatorSuite) TestCharm(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASOperator")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "Charm")
		c.Check(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{
				Tag: "application-gitlab",
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ApplicationCharmResults{})
		*(result.(*params.ApplicationCharmResults)) = params.ApplicationCharmResults{
			Results: []params.ApplicationCharmResult{{
				Result: &params.ApplicationCharm{
					URL:                  "cs:foo/bar-1",
					ForceUpgrade:         true,
					SHA256:               "fake-sha256",
					CharmModifiedVersion: 666,
				},
			}},
		}
		return nil
	})

	client := caasoperator.NewClient(apiCaller)
	curl, force, sha256, vers, err := client.Charm("gitlab")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(curl, gc.NotNil)
	c.Assert(curl.String(), gc.Equals, "cs:foo/bar-1")
	c.Assert(sha256, gc.Equals, "fake-sha256")
	c.Assert(force, jc.IsTrue)
	c.Assert(vers, gc.Equals, 666)
}

func (s *operatorSuite) TestCharmError(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.ApplicationCharmResults)) = params.ApplicationCharmResults{
			Results: []params.ApplicationCharmResult{{Error: &params.Error{Message: "bletch"}}},
		}
		return nil
	})
	client := caasoperator.NewClient(apiCaller)
	_, _, _, _, err := client.Charm("gitlab")
	c.Assert(err, gc.ErrorMatches, "bletch")
}

func (s *operatorSuite) TestCharmInvalidApplicationName(c *gc.C) {
	client := caasoperator.NewClient(basetesting.APICallerFunc(func(_ string, _ int, _, _ string, _, _ interface{}) error {
		return errors.New("should not be called")
	}))
	_, _, _, _, err := client.Charm("")
	c.Assert(err, gc.ErrorMatches, `application name "" not valid`)
}

func (s *operatorSuite) TestSetPodSpec(c *gc.C) {
	tag := names.NewApplicationTag("gitlab")
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASOperator")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "SetPodSpec")
		c.Check(arg, jc.DeepEquals, params.SetPodSpecParams{
			Specs: []params.EntityString{{
				Tag:   tag.String(),
				Value: "spec",
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: &params.Error{Message: "bletch"}}},
		}
		return nil
	})

	client := caasoperator.NewClient(apiCaller)
	err := client.SetPodSpec(tag.Id(), "spec")
	c.Assert(err, gc.ErrorMatches, "bletch")
}

func (s *operatorSuite) TestSetPodSpecInvalidEntityame(c *gc.C) {
	client := caasoperator.NewClient(basetesting.APICallerFunc(func(_ string, _ int, _, _ string, _, _ interface{}) error {
		return errors.New("should not be called")
	}))
	err := client.SetPodSpec("", "spec")
	c.Assert(err, gc.ErrorMatches, `application name "" not valid`)
}

func (s *operatorSuite) TestModel(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASOperator")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "CurrentModel")
		c.Check(arg, gc.IsNil)
		c.Assert(result, gc.FitsTypeOf, &params.ModelResult{})
		*(result.(*params.ModelResult)) = params.ModelResult{
			Name: "some-model",
			UUID: "deadbeef",
			Type: "iaas",
		}
		return nil
	})

	client := caasoperator.NewClient(apiCaller)
	m, err := client.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m, jc.DeepEquals, &model.Model{
		Name:      "some-model",
		UUID:      "deadbeef",
		ModelType: model.IAAS,
	})
}

func (s *operatorSuite) TestWatchUnits(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASOperator")
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

	client := caasoperator.NewClient(apiCaller)
	watcher, err := client.WatchUnits("gitlab")
	c.Assert(watcher, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "FAIL")
}

func (s *operatorSuite) TestUnits(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASOperator")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "UnitsStatus")
		c.Assert(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{
				Tag: "unit-gitlab-0",
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.UnitStatusResults{})
		*(result.(*params.UnitStatusResults)) = params.UnitStatusResults{
			Results: []params.UnitStatusResult{{
				Result: &params.UnitStatus{
					ProviderId: "gitlab-xxxx", Address: "1.1.1.1",
				},
			}},
		}
		return nil
	})

	client := caasoperator.NewClient(apiCaller)
	result, err := client.UnitsStatus(names.NewUnitTag("gitlab/0"))
	c.Assert(err, gc.IsNil)
	c.Logf("%+v", result.Results[0].Result)
	c.Assert(result, gc.DeepEquals, params.UnitStatusResults{
		Results: []params.UnitStatusResult{{
			Result: &params.UnitStatus{
				ProviderId: "gitlab-xxxx", Address: "1.1.1.1",
			},
		}},
	})
}

func (s *operatorSuite) TestRemoveUnit(c *gc.C) {
	called := false
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASOperator")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "Remove")
		c.Check(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{
				Tag: "unit-gitlab-0",
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		called = true
		return nil
	})

	client := caasoperator.NewClient(apiCaller)
	err := client.RemoveUnit("gitlab/0")
	c.Assert(err, gc.ErrorMatches, "FAIL")
	c.Assert(called, jc.IsTrue)
}

func (s *operatorSuite) TestRemoveUnitInvalidUnitName(c *gc.C) {
	client := caasoperator.NewClient(basetesting.APICallerFunc(func(_ string, _ int, _, _ string, _, _ interface{}) error {
		return errors.New("should not be called")
	}))
	err := client.RemoveUnit("")
	c.Assert(err, gc.ErrorMatches, `unit name "" not valid`)
}

func (s *operatorSuite) TestLife(c *gc.C) {
	s.testLife(c, names.NewApplicationTag("gitlab"))
	s.testLife(c, names.NewUnitTag("gitlab/0"))
}

func (s *operatorSuite) testLife(c *gc.C, tag names.Tag) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASOperator")
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

	client := caasoperator.NewClient(apiCaller)
	lifeValue, err := client.Life(tag.Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(lifeValue, gc.Equals, life.Alive)
}

func (s *operatorSuite) TestLifeError(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.LifeResults)) = params.LifeResults{
			Results: []params.LifeResult{{Error: &params.Error{
				Code:    params.CodeNotFound,
				Message: "bletch",
			}}},
		}
		return nil
	})

	client := caasoperator.NewClient(apiCaller)
	_, err := client.Life("gitlab/0")
	c.Assert(err, gc.ErrorMatches, "bletch")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *operatorSuite) TestLifeInvalidEntityName(c *gc.C) {
	client := caasoperator.NewClient(basetesting.APICallerFunc(func(_ string, _ int, _, _ string, _, _ interface{}) error {
		return errors.New("should not be called")
	}))
	_, err := client.Life("")
	c.Assert(err, gc.ErrorMatches, `application or unit name "" not valid`)
}

func (s *operatorSuite) TestSetVersion(c *gc.C) {
	called := false
	vers := version.Binary{}
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASOperator")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "SetTools")
		c.Check(arg, jc.DeepEquals, params.EntitiesVersion{
			AgentTools: []params.EntityVersion{{
				Tag:   "application-gitlab",
				Tools: &params.Version{Version: vers},
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		called = true
		return nil
	})

	client := caasoperator.NewClient(apiCaller)
	err := client.SetVersion("gitlab", vers)
	c.Assert(err, gc.ErrorMatches, "FAIL")
	c.Assert(called, jc.IsTrue)
}

func (s *operatorSuite) TestSetVersionInvalidApplicationName(c *gc.C) {
	client := caasoperator.NewClient(basetesting.APICallerFunc(func(_ string, _ int, _, _ string, _, _ interface{}) error {
		return errors.New("should not be called")
	}))
	err := client.SetVersion("", version.Binary{})
	c.Assert(err, gc.ErrorMatches, `application name "" not valid`)
}
