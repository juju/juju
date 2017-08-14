// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/agent"
	apitesting "github.com/juju/juju/api/testing"
	jujutesting "github.com/juju/juju/juju/testing"
)

type modelSuite struct {
	jujutesting.JujuConnSuite
	*apitesting.ModelWatcherTests
}

var _ = gc.Suite(&modelSuite{})

func (s *modelSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	stateAPI, _ := s.OpenAPIAsNewMachine(c)

	agentAPI, err := agent.NewState(stateAPI)
	c.Assert(agentAPI, gc.NotNil)
	c.Assert(err, jc.ErrorIsNil)

	s.ModelWatcherTests = apitesting.NewModelWatcherTests(
		agentAPI, s.BackingState,
	)
}
