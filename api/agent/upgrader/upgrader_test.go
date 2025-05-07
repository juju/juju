// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader_test

import (
	"context"

	"github.com/juju/tc"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/api/agent/upgrader"
	"github.com/juju/juju/api/base/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/tools"
	"github.com/juju/juju/rpc/params"
)

type machineUpgraderSuite struct {
	jujutesting.IsolationSuite
}

var _ = tc.Suite(&machineUpgraderSuite{})

func (s *machineUpgraderSuite) TestSetVersion(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Upgrader")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "SetTools")
		c.Check(arg, jc.DeepEquals, params.EntitiesVersion{
			AgentTools: []params.EntityVersion{{
				Tag:   "machine-666",
				Tools: &params.Version{Version: coretesting.CurrentVersion()},
			}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: &params.Error{Message: "FAIL"}}},
		}
		return nil

	})
	client := upgrader.NewClient(apiCaller)
	err := client.SetVersion(context.Background(), "machine-666", coretesting.CurrentVersion())
	c.Assert(err, tc.ErrorMatches, "FAIL")
}

func (s *machineUpgraderSuite) TestTools(c *tc.C) {
	toolsResult := tools.List{{URL: "https://tools"}}
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Upgrader")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "Tools")
		c.Check(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "machine-666"}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ToolsResults{})
		*(result.(*params.ToolsResults)) = params.ToolsResults{
			Results: []params.ToolsResult{{ToolsList: toolsResult}},
		}
		return nil

	})
	client := upgrader.NewClient(apiCaller)
	t, err := client.Tools(context.Background(), "machine-666")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(t, jc.DeepEquals, toolsResult)
}

func (s *machineUpgraderSuite) TestWatchAPIVersion(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Upgrader")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "WatchAPIVersion")
		c.Check(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "machine-666"}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.NotifyWatchResults{})
		*(result.(*params.NotifyWatchResults)) = params.NotifyWatchResults{
			Results: []params.NotifyWatchResult{{Error: &params.Error{Message: "FAIL"}}},
		}
		return nil

	})
	client := upgrader.NewClient(apiCaller)
	_, err := client.WatchAPIVersion(context.Background(), "machine-666")
	c.Assert(err, tc.ErrorMatches, "FAIL")
}

func (s *machineUpgraderSuite) TestDesiredVersion(c *tc.C) {
	versResult := coretesting.CurrentVersion().Number
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, tc.Equals, "Upgrader")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "DesiredVersion")
		c.Check(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "machine-666"}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.VersionResults{})
		*(result.(*params.VersionResults)) = params.VersionResults{
			Results: []params.VersionResult{{Version: &versResult}},
		}
		return nil

	})
	client := upgrader.NewClient(apiCaller)
	v, err := client.DesiredVersion(context.Background(), "machine-666")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(v, jc.DeepEquals, versResult)
}
