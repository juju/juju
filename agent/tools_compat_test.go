// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
)

//TODO: When we get rid of *state.Tools, we won't need this test suite.
//      This is used to ensure state.Tools is compatible with agent.Tools so we
//      can migrate code over to agent.Tools and cast when needed
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
