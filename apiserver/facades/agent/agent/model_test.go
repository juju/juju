// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	commontesting "github.com/juju/juju/apiserver/common/testing"
	"github.com/juju/juju/apiserver/facades/agent/agent"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
)

type modelSuite struct {
	testing.ApiServerSuite
	*commontesting.ModelWatcherTest

	authorizer apiservertesting.FakeAuthorizer
	resources  *common.Resources

	machine0 *state.Machine
	api      *agent.AgentAPI
}

var _ = gc.Suite(&modelSuite{})

func (s *modelSuite) SetUpTest(c *gc.C) {
	s.ApiServerSuite.SetUpTest(c)

	st := s.ControllerModel(c).State()
	var err error
	s.machine0, err = st.AddMachine(s.InstancePrechecker(c, st), state.UbuntuBase("12.10"), state.JobHostUnits, state.JobManageModel)
	c.Assert(err, jc.ErrorIsNil)

	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.machine0.Tag(),
	}
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	s.api, err = agent.NewAgentAPI(
		s.authorizer,
		s.resources,
		s.ControllerModel(c).State(),
		nil,
		nil,
		nil,
		nil,
	)
	c.Assert(err, jc.ErrorIsNil)
	s.ModelWatcherTest = commontesting.NewModelWatcherTest(
		s.api, st, s.resources,
	)
}
