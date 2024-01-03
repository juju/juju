// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/caasfirewaller"
	"github.com/juju/juju/core/config"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

type clientCommmon interface {
	WatchApplications() (watcher.StringsWatcher, error)
	WatchApplication(string) (watcher.NotifyWatcher, error)
	IsExposed(string) (bool, error)
	ApplicationConfig(string) (config.ConfigAttributes, error)
	Life(string) (life.Value, error)
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

func (s *firewallerSuite) TestWatchOpenedPorts(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, s.objType)
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchOpenedPorts")
		c.Check(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{
				Tag: "model-deadbeef-0bad-400d-8000-4b1d0d06f00d",
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

	client := caasfirewaller.NewClient(apiCaller)
	watcher, err := client.WatchOpenedPorts()
	c.Assert(watcher, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "FAIL")
}

func (s *firewallerSuite) TestGetOpenedPorts(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, s.objType)
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "GetOpenedPorts")
		c.Check(arg, jc.DeepEquals, params.Entity{Tag: "application-gitlab"})
		c.Assert(result, gc.FitsTypeOf, &params.ApplicationOpenedPortsResults{})
		*(result.(*params.ApplicationOpenedPortsResults)) = params.ApplicationOpenedPortsResults{
			Results: []params.ApplicationOpenedPortsResult{{
				ApplicationPortRanges: []params.ApplicationOpenedPorts{
					{
						PortRanges: []params.PortRange{
							{
								FromPort: 80,
								ToPort:   8080,
								Protocol: "tcp",
							},
						},
					},
				},
			}},
		}
		return nil
	})

	client := caasfirewaller.NewClient(apiCaller)
	result, err := client.GetOpenedPorts("gitlab")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, network.GroupedPortRanges{
		"": []network.PortRange{
			{
				FromPort: 80,
				ToPort:   8080,
				Protocol: "tcp",
			},
		},
	})
}

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
	exposed, err := client.IsExposed("gitlab")
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
	_, err := client.IsExposed("gitlab")
	c.Assert(err, gc.ErrorMatches, "bletch")
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *firewallerSuite) TestIsExposedInvalidEntityame(c *gc.C) {
	client := s.newFunc(basetesting.APICallerFunc(func(_ string, _ int, _, _ string, _, _ interface{}) error {
		return errors.New("should not be called")
	}))
	_, err := client.IsExposed("")
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
	lifeValue, err := client.Life(tag.Id())
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
	_, err := client.Life("gitlab")
	c.Assert(err, gc.ErrorMatches, "bletch")
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *firewallerSuite) TestLifeInvalidEntityame(c *gc.C) {
	client := s.newFunc(basetesting.APICallerFunc(func(_ string, _ int, _, _ string, _, _ interface{}) error {
		return errors.New("should not be called")
	}))
	_, err := client.Life("")
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
	watcher, err := client.WatchApplications()
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
	watcher, err := client.WatchApplication("gitlab")
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
	cfg, err := client.ApplicationConfig("gitlab")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg, jc.DeepEquals, config.ConfigAttributes{"foo": "bar"})
}
