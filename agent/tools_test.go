// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent_test

import (
	stdtesting "testing"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/agent"
)

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

var _ = gc.Suite(&ToolsSuite{})

type ToolsSuite struct {
	coretesting.LoggingSuite
}

func (s *ToolsSuite) TestToolsMatchStateTools(c *gc.C) {
	testtools := agent.Tools{}
	statetools := state.Tools(testtools)
	testtools2 := agent.Tools(statetools)
	c.Assert(testtools, gc.Equals, testtools2)
}

func (s *ToolsSuite) TestToolPointers(c *gc.C) {
	testtools := &agent.Tools{}
	statetools := (*state.Tools)(testtools)
	testtools2 := (*agent.Tools)(statetools)
	c.Assert(testtools, gc.Equals, testtools2)
}
