// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent_test

import (
	"github.com/juju/clock"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	commontesting "github.com/juju/juju/apiserver/common/testing"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/agent/agent"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/watcher/registry"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
)

type modelSuite struct {
	testing.JujuConnSuite
	*commontesting.ModelWatcherTest

	authorizer      apiservertesting.FakeAuthorizer
	watcherRegistry facade.WatcherRegistry

	machine0 *state.Machine
	api      *agent.AgentAPI
}

var _ = gc.Suite(&modelSuite{})

func (s *modelSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	var err error
	s.watcherRegistry, err = registry.NewRegistry(clock.WallClock)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) { workertest.DirtyKill(c, s.watcherRegistry) })

	s.machine0, err = s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits, state.JobManageModel)
	c.Assert(err, jc.ErrorIsNil)

	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.machine0.Tag(),
	}

	s.api, err = agent.NewAgentAPIV3(facadetest.Context{
		State_:           s.State,
		WatcherRegistry_: s.watcherRegistry,
		Auth_:            s.authorizer,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.ModelWatcherTest = commontesting.NewModelWatcherTest(
		s.api, s.State, s.watcherRegistry,
	)
}
