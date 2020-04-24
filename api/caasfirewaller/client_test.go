// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/caasfirewaller"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/life"
)

type FirewallerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&FirewallerSuite{})

func (s *FirewallerSuite) TestIsExposed(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASFirewaller")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "IsExposed")
		c.Check(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{
				Tag: "application-gitlab",
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.BoolResults{})
		*(result.(*params.BoolResults)) = params.BoolResults{
			Results: []params.BoolResult{{
				Result: true,
			}},
		}
		return nil
	})

	client := caasfirewaller.NewClient(apiCaller)
	exposed, err := client.IsExposed("gitlab")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(exposed, jc.IsTrue)
}

func (s *FirewallerSuite) TestIsExposedError(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.BoolResults)) = params.BoolResults{
			Results: []params.BoolResult{{Error: &params.Error{
				Code:    params.CodeNotFound,
				Message: "bletch",
			}}},
		}
		return nil
	})

	client := caasfirewaller.NewClient(apiCaller)
	_, err := client.IsExposed("gitlab")
	c.Assert(err, gc.ErrorMatches, "bletch")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *FirewallerSuite) TestIsExposedInvalidEntityame(c *gc.C) {
	client := caasfirewaller.NewClient(basetesting.APICallerFunc(func(_ string, _ int, _, _ string, _, _ interface{}) error {
		return errors.New("should not be called")
	}))
	_, err := client.IsExposed("")
	c.Assert(err, gc.ErrorMatches, `application name "" not valid`)
}

func (s *FirewallerSuite) TestLife(c *gc.C) {
	tag := names.NewApplicationTag("gitlab")
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASFirewaller")
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

	client := caasfirewaller.NewClient(apiCaller)
	lifeValue, err := client.Life(tag.Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(lifeValue, gc.Equals, life.Alive)
}

func (s *FirewallerSuite) TestLifeError(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.LifeResults)) = params.LifeResults{
			Results: []params.LifeResult{{Error: &params.Error{
				Code:    params.CodeNotFound,
				Message: "bletch",
			}}},
		}
		return nil
	})

	client := caasfirewaller.NewClient(apiCaller)
	_, err := client.Life("gitlab")
	c.Assert(err, gc.ErrorMatches, "bletch")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *FirewallerSuite) TestLifeInvalidEntityame(c *gc.C) {
	client := caasfirewaller.NewClient(basetesting.APICallerFunc(func(_ string, _ int, _, _ string, _, _ interface{}) error {
		return errors.New("should not be called")
	}))
	_, err := client.Life("")
	c.Assert(err, gc.ErrorMatches, `application name "" not valid`)
}

func (s *FirewallerSuite) TestWatchApplications(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASFirewaller")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchApplications")
		c.Assert(result, gc.FitsTypeOf, &params.StringsWatchResult{})
		*(result.(*params.StringsWatchResult)) = params.StringsWatchResult{
			Error: &params.Error{Message: "FAIL"},
		}
		return nil
	})

	client := caasfirewaller.NewClient(apiCaller)
	watcher, err := client.WatchApplications()
	c.Assert(watcher, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "FAIL")
}

func (s *FirewallerSuite) TestWatchApplication(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASFirewaller")
		c.Check(version, gc.Equals, 0)
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

	client := caasfirewaller.NewClient(apiCaller)
	watcher, err := client.WatchApplication("gitlab")
	c.Assert(watcher, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "FAIL")
}

func (s *FirewallerSuite) TestApplicationConfig(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASFirewaller")
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

	client := caasfirewaller.NewClient(apiCaller)
	cfg, err := client.ApplicationConfig("gitlab")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg, jc.DeepEquals, application.ConfigAttributes{"foo": "bar"})
}
