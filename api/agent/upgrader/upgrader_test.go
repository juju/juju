// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader_test

import (
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/agent/upgrader"
	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/internal/tools"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

type machineUpgraderSuite struct {
	jujutesting.IsolationSuite
}

var _ = gc.Suite(&machineUpgraderSuite{})

func (s *machineUpgraderSuite) TestSetVersion(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Upgrader")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "SetTools")
		c.Check(arg, jc.DeepEquals, params.EntitiesVersion{
			AgentTools: []params.EntityVersion{{
				Tag:   "machine-666",
				Tools: &params.Version{Version: coretesting.CurrentVersion()},
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: &params.Error{Message: "FAIL"}}},
		}
		return nil

	})
	client := upgrader.NewClient(apiCaller)
	err := client.SetVersion("machine-666", coretesting.CurrentVersion())
	c.Assert(err, gc.ErrorMatches, "FAIL")
}

func (s *machineUpgraderSuite) TestTools(c *gc.C) {
	toolsResult := tools.List{{URL: "https://tools"}}
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Upgrader")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "Tools")
		c.Check(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "machine-666"}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ToolsResults{})
		*(result.(*params.ToolsResults)) = params.ToolsResults{
			Results: []params.ToolsResult{{ToolsList: toolsResult}},
		}
		return nil

	})
	client := upgrader.NewClient(apiCaller)
	t, err := client.Tools("machine-666")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(t, jc.DeepEquals, toolsResult)
}

func (s *machineUpgraderSuite) TestWatchAPIVersion(c *gc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Upgrader")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchAPIVersion")
		c.Check(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "machine-666"}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.NotifyWatchResults{})
		*(result.(*params.NotifyWatchResults)) = params.NotifyWatchResults{
			Results: []params.NotifyWatchResult{{Error: &params.Error{Message: "FAIL"}}},
		}
		return nil

	})
	client := upgrader.NewClient(apiCaller)
	_, err := client.WatchAPIVersion("machine-666")
	c.Assert(err, gc.ErrorMatches, "FAIL")
}

func (s *machineUpgraderSuite) TestDesiredVersion(c *gc.C) {
	versResult := coretesting.CurrentVersion().Number
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Upgrader")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "DesiredVersion")
		c.Check(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "machine-666"}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.VersionResults{})
		*(result.(*params.VersionResults)) = params.VersionResults{
			Results: []params.VersionResult{{Version: &versResult}},
		}
		return nil

	})
	client := upgrader.NewClient(apiCaller)
	v, err := client.DesiredVersion("machine-666")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(v, jc.DeepEquals, versResult)
}
