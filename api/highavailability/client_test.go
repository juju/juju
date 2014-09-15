// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package highavailability_test

import (
	stdtesting "testing"

	gc "launchpad.net/gocheck"

	"github.com/juju/juju/api/highavailability"
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

func (s *clientSuite) TestClientEnsureAvailabilityFailsBadEnvTag(c *gc.C) {
	_, err := s.State.AddMachine("quantal", state.JobManageEnviron)
	c.Assert(err, gc.IsNil)

	emptyCons := constraints.Value{}
	defaultSeries := ""
	client := highavailability.NewClient(s.APIState, "bad-env-uuid")
	_, err = client.EnsureAvailability(3, emptyCons, defaultSeries, nil)
	c.Assert(err, gc.ErrorMatches,
		`invalid environment tag: "bad-env-uuid" is not a valid tag`)
}

type Killer interface {
	Kill() error
}

func assertKill(c *gc.C, killer Killer) {
	c.Assert(killer.Kill(), gc.IsNil)
}

func (s *clientSuite) setAgentPresence(c *gc.C, machineId string) *presence.Pinger {
	m, err := s.BackingState.Machine(machineId)
	c.Assert(err, gc.IsNil)
	pinger, err := m.SetAgentPresence()
	c.Assert(err, gc.IsNil)
	s.BackingState.StartSync()
	err = m.WaitAgentPresence(coretesting.LongWait)
	c.Assert(err, gc.IsNil)
	return pinger
}

func (s *clientSuite) TestClientEnsureAvailability(c *gc.C) {
	_, err := s.State.AddMachine("quantal", state.JobManageEnviron)
	c.Assert(err, gc.IsNil)
	// We have to ensure the agents are alive, or EnsureAvailability will
	// create more to replace them.
	pingerA := s.setAgentPresence(c, "0")
	defer assertKill(c, pingerA)

	emptyCons := constraints.Value{}
	result, err := highavailability.NewClient(
		s.APIState, s.State.EnvironTag().String()).EnsureAvailability(3, emptyCons, "", nil)
	c.Assert(err, gc.IsNil)

	c.Assert(result.Maintained, gc.DeepEquals, []string{"machine-0"})
	c.Assert(result.Added, gc.DeepEquals, []string{"machine-1", "machine-2"})
	c.Assert(result.Removed, gc.HasLen, 0)

	machines, err := s.State.AllMachines()
	c.Assert(err, gc.IsNil)
	c.Assert(machines, gc.HasLen, 3)
	c.Assert(machines[0].Series(), gc.Equals, "quantal")
	c.Assert(machines[1].Series(), gc.Equals, "quantal")
	c.Assert(machines[2].Series(), gc.Equals, "quantal")
}
