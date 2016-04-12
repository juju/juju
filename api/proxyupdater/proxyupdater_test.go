// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater_test

import (
	gc "gopkg.in/check.v1"

	apiagent "github.com/juju/juju/api/agent"
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

	conn, _ := s.OpenAPIAsNewMachine(c)
	agentAPI := apiagent.NewState(conn)
	c.Assert(agentAPI, gc.NotNil)

	s.ModelWatcherTests = apitesting.NewModelWatcherTests(
		agentAPI, s.BackingState, apitesting.NoSecrets)
}
