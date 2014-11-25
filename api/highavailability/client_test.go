// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package highavailability_test

import (
	stdtesting "testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/highavailability"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/constraints"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/presence"
	coretesting "github.com/juju/juju/testing"
)

func TestAll(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type clientSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&clientSuite{})

type Killer interface {
	Kill() error
}

func assertKill(c *gc.C, killer Killer) {
	c.Assert(killer.Kill(), gc.IsNil)
}

func setAgentPresence(c *gc.C, s *jujutesting.JujuConnSuite, machineId string) *presence.Pinger {
	m, err := s.BackingState.Machine(machineId)
	c.Assert(err, jc.ErrorIsNil)
	pinger, err := m.SetAgentPresence()
	c.Assert(err, jc.ErrorIsNil)
	s.BackingState.StartSync()
	err = m.WaitAgentPresence(coretesting.LongWait)
	c.Assert(err, jc.ErrorIsNil)
	return pinger
}

func assertEnsureAvailability(c *gc.C, s *jujutesting.JujuConnSuite) {
	_, err := s.State.AddMachine("quantal", state.JobManageEnviron)
	c.Assert(err, jc.ErrorIsNil)
	// We have to ensure the agents are alive, or EnsureAvailability will
	// create more to replace them.
	pingerA := setAgentPresence(c, s, "0")
	defer assertKill(c, pingerA)

	emptyCons := constraints.Value{}
	client := highavailability.NewClient(s.APIState)
	result, err := client.EnsureAvailability(3, emptyCons, "", nil)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(result.Maintained, gc.DeepEquals, []string{"machine-0"})
	c.Assert(result.Added, gc.DeepEquals, []string{"machine-1", "machine-2"})
	c.Assert(result.Removed, gc.HasLen, 0)

	machines, err := s.State.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 3)
	c.Assert(machines[0].Series(), gc.Equals, "quantal")
	c.Assert(machines[1].Series(), gc.Equals, "quantal")
	c.Assert(machines[2].Series(), gc.Equals, "quantal")
}

func (s *clientSuite) TestClientEnsureAvailability(c *gc.C) {
	assertEnsureAvailability(c, &s.JujuConnSuite)
}

func (s *clientSuite) TestClientEnsureAvailabilityVersion(c *gc.C) {
	client := highavailability.NewClient(s.APIState)
	c.Assert(client.BestAPIVersion(), gc.Equals, 1)
}

type clientLegacySuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&clientLegacySuite{})

func (s *clientLegacySuite) SetUpTest(c *gc.C) {
	common.Facades.Discard("HighAvailability", 1)
	s.JujuConnSuite.SetUpTest(c)
}

func (s *clientLegacySuite) TestEnsureAvailabilityLegacy(c *gc.C) {
	assertEnsureAvailability(c, &s.JujuConnSuite)
}

func (s *clientLegacySuite) TestEnsureAvailabilityLegacyRejectsPlacement(c *gc.C) {
	client := highavailability.NewClient(s.APIState)
	_, err := client.EnsureAvailability(3, constraints.Value{}, "", []string{"machine"})
	c.Assert(err, gc.ErrorMatches, "placement directives not supported with this version of Juju")
}
