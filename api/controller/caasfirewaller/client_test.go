// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/caasfirewaller"
	"github.com/juju/juju/core/config"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

type clientCommmon interface {
	WatchApplications(context.Context) (watcher.StringsWatcher, error)
	WatchApplication(context.Context, string) (watcher.NotifyWatcher, error)
	IsExposed(context.Context, string) (bool, error)
	ApplicationConfig(context.Context, string) (config.ConfigAttributes, error)
	Life(context.Context, string) (life.Value, error)
}

type firewallerSuite struct {
	testing.IsolationSuite

	newFunc func(caller base.APICaller) clientCommmon
	objType string
}

var _ = gc.Suite(&firewallerSuite{
	objType: "CAASFirewaller",
	newFunc: func(caller base.APICaller) clientCommmon {
		return caasfirewaller.NewClient(caller)
	},
})

func (s *firewallerSuite) TestIsExposed(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, s.objType)
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

	client := s.newFunc(apiCaller)
	exposed, err := client.IsExposed(context.Background(), "gitlab")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(exposed, jc.IsTrue)
}

func (s *firewallerSuite) TestIsExposedError(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.BoolResults)) = params.BoolResults{
			Results: []params.BoolResult{{Error: &params.Error{
				Code:    params.CodeNotFound,
				Message: "bletch",
			}}},
		}
		return nil
	})

	client := s.newFunc(apiCaller)
	_, err := client.IsExposed(context.Background(), "gitlab")
	c.Assert(err, gc.ErrorMatches, "bletch")
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *firewallerSuite) TestIsExposedInvalidEntityame(c *gc.C) {
	client := s.newFunc(basetesting.APICallerFunc(func(_ string, _ int, _, _ string, _, _ interface{}) error {
		return errors.New("should not be called")
	}))
	_, err := client.IsExposed(context.Background(), "")
	c.Assert(err, gc.ErrorMatches, `application name "" not valid`)
}

func (s *firewallerSuite) TestLife(c *gc.C) {
	tag := names.NewApplicationTag("gitlab")
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, s.objType)
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

	client := s.newFunc(apiCaller)
	lifeValue, err := client.Life(context.Background(), tag.Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(lifeValue, gc.Equals, life.Alive)
}

func (s *firewallerSuite) TestLifeError(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.LifeResults)) = params.LifeResults{
			Results: []params.LifeResult{{Error: &params.Error{
				Code:    params.CodeNotFound,
				Message: "bletch",
			}}},
		}
		return nil
	})

	client := s.newFunc(apiCaller)
	_, err := client.Life(context.Background(), "gitlab")
	c.Assert(err, gc.ErrorMatches, "bletch")
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *firewallerSuite) TestLifeInvalidEntityame(c *gc.C) {
	client := s.newFunc(basetesting.APICallerFunc(func(_ string, _ int, _, _ string, _, _ interface{}) error {
		return errors.New("should not be called")
	}))
	_, err := client.Life(context.Background(), "")
	c.Assert(err, gc.ErrorMatches, `application name "" not valid`)
}

func (s *firewallerSuite) TestWatchApplications(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, s.objType)
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchApplications")
		c.Assert(result, gc.FitsTypeOf, &params.StringsWatchResult{})
		*(result.(*params.StringsWatchResult)) = params.StringsWatchResult{
			Error: &params.Error{Message: "FAIL"},
		}
		return nil
	})

	client := s.newFunc(apiCaller)
	watcher, err := client.WatchApplications(context.Background())
	c.Assert(watcher, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "FAIL")
}

func (s *firewallerSuite) TestWatchApplication(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, s.objType)
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

	client := s.newFunc(apiCaller)
	watcher, err := client.WatchApplication(context.Background(), "gitlab")
	c.Assert(watcher, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "FAIL")
}

func (s *firewallerSuite) TestApplicationConfig(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, s.objType)
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

	client := s.newFunc(apiCaller)
	cfg, err := client.ApplicationConfig(context.Background(), "gitlab")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg, jc.DeepEquals, config.ConfigAttributes{"foo": "bar"})
}
