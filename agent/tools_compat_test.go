// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
)

var _ = gc.Suite(&ToolsCompatSuite{})

type ToolsCompatSuite struct {
	coretesting.LoggingSuite
}

func (s *ToolsCompatSuite) TestToolsMatchStateTools(c *gc.C) {
	testtools := agent.Tools{}
	statetools := state.Tools(testtools)
	testtools2 := agent.Tools(statetools)
	c.Assert(testtools, gc.Equals, testtools2)
}

func (s *ToolsCompatSuite) TestToolPointers(c *gc.C) {
	testtools := &agent.Tools{}
	statetools := (*state.Tools)(testtools)
	testtools2 := (*agent.Tools)(statetools)
	c.Assert(testtools, gc.Equals, testtools2)
}
