// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v3/apiserver/common"
	commontesting "github.com/juju/juju/v3/apiserver/common/testing"
	"github.com/juju/juju/v3/apiserver/facade/facadetest"
	"github.com/juju/juju/v3/apiserver/facades/agent/agent"
	apiservertesting "github.com/juju/juju/v3/apiserver/testing"
	"github.com/juju/juju/v3/juju/testing"
	"github.com/juju/juju/v3/state"
)

type modelSuite struct {
	testing.JujuConnSuite
	*commontesting.ModelWatcherTest

	authorizer apiservertesting.FakeAuthorizer
	resources  *common.Resources

	machine0 *state.Machine
	api      *agent.AgentAPI
}

var _ = gc.Suite(&modelSuite{})

func (s *modelSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	var err error
	s.machine0, err = s.State.AddMachine("quantal", state.JobHostUnits, state.JobManageModel)
	c.Assert(err, jc.ErrorIsNil)

	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.machine0.Tag(),
	}
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	s.api, err = agent.NewAgentAPIV3(facadetest.Context{
		State_:     s.State,
		Resources_: s.resources,
		Auth_:      s.authorizer,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.ModelWatcherTest = commontesting.NewModelWatcherTest(
		s.api, s.State, s.resources,
	)
}
