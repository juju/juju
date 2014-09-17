// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package highavailability_test

import (
	stdtesting "testing"

	"github.com/juju/names"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/highavailability"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/presence"
	coretesting "github.com/juju/juju/testing"
)

func TestAll(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type clientSuite struct {
	testing.JujuConnSuite

	resources  *common.Resources
	authoriser apiservertesting.FakeAuthorizer
	haServer   *highavailability.HighAvailabilityAPI
	pinger     *presence.Pinger
}

type Killer interface {
	Kill() error
}

var _ = gc.Suite(&clientSuite{})

func assertKill(c *gc.C, killer Killer) {
	c.Assert(killer.Kill(), gc.IsNil)
}

var (
	emptyCons     = constraints.Value{}
	defaultSeries = ""
)

func (s *clientSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	s.authoriser = apiservertesting.FakeAuthorizer{
		Tag:            names.NewUserTag(state.AdminUser),
		EnvironManager: true,
	}

	var err error
	s.haServer, err = highavailability.NewHighAvailabilityAPI(s.State, s.resources, s.authoriser)
	c.Assert(err, gc.IsNil)

	_, err = s.State.AddMachine("quantal", state.JobManageEnviron)
	c.Assert(err, gc.IsNil)
	// We have to ensure the agents are alive, or EnsureAvailability will
	// create more to replace them.
	s.pinger = s.setAgentPresence(c, "0")
}

func (s *clientSuite) TearDownTest(c *gc.C) {
	assertKill(c, s.pinger)
	s.JujuConnSuite.TearDownTest(c)
}

func (s *clientSuite) setAgentPresence(c *gc.C, machineId string) *presence.Pinger {
	m, err := s.State.Machine(machineId)
	c.Assert(err, gc.IsNil)
	pinger, err := m.SetAgentPresence()
	c.Assert(err, gc.IsNil)
	s.State.StartSync()
	err = m.WaitAgentPresence(coretesting.LongWait)
	c.Assert(err, gc.IsNil)
	return pinger
}

func (s *clientSuite) ensureAvailability(
	c *gc.C, numStateServers int, cons constraints.Value, series string, placement []string,
) (params.StateServersChanges, error) {

	arg := params.StateServersSpecs{
		Specs: []params.StateServersSpec{{
			NumStateServers: numStateServers,
			Constraints:     cons,
			Series:          series,
			Placement:       placement,
		}}}
	results, err := s.haServer.EnsureAvailability(arg)
	c.Assert(err, gc.IsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	result := results.Results[0]
	return result.Result, result.Error
}

func (s *clientSuite) TestEnsureAvailabilitySeries(c *gc.C) {
	machines, err := s.State.AllMachines()
	c.Assert(err, gc.IsNil)
	c.Assert(machines, gc.HasLen, 1)
	c.Assert(machines[0].Series(), gc.Equals, "quantal")

	ensureAvailabilityResult, err := s.ensureAvailability(c, 3, emptyCons, defaultSeries, nil)
	c.Assert(err, gc.IsNil)
	c.Assert(ensureAvailabilityResult.Maintained, gc.DeepEquals, []string{"machine-0"})
	c.Assert(ensureAvailabilityResult.Added, gc.DeepEquals, []string{"machine-1", "machine-2"})
	c.Assert(ensureAvailabilityResult.Removed, gc.HasLen, 0)

	machines, err = s.State.AllMachines()
	c.Assert(err, gc.IsNil)
	c.Assert(machines, gc.HasLen, 3)
	c.Assert(machines[0].Series(), gc.Equals, "quantal")
	c.Assert(machines[1].Series(), gc.Equals, "quantal")
	c.Assert(machines[2].Series(), gc.Equals, "quantal")

	pingerB := s.setAgentPresence(c, "1")
	defer assertKill(c, pingerB)

	pingerC := s.setAgentPresence(c, "2")
	defer assertKill(c, pingerC)

	ensureAvailabilityResult, err = s.ensureAvailability(c, 5, emptyCons, "non-default", nil)
	c.Assert(err, gc.IsNil)
	c.Assert(ensureAvailabilityResult.Maintained, gc.DeepEquals, []string{"machine-0", "machine-1", "machine-2"})
	c.Assert(ensureAvailabilityResult.Added, gc.DeepEquals, []string{"machine-3", "machine-4"})
	c.Assert(ensureAvailabilityResult.Removed, gc.HasLen, 0)

	c.Assert(err, gc.IsNil)
	machines, err = s.State.AllMachines()
	c.Assert(err, gc.IsNil)
	c.Assert(machines, gc.HasLen, 5)
	c.Assert(machines[0].Series(), gc.Equals, "quantal")
	c.Assert(machines[1].Series(), gc.Equals, "quantal")
	c.Assert(machines[2].Series(), gc.Equals, "quantal")
	c.Assert(machines[3].Series(), gc.Equals, "non-default")
	c.Assert(machines[4].Series(), gc.Equals, "non-default")
}

func (s *clientSuite) TestEnsureAvailabilityConstraints(c *gc.C) {
	ensureAvailabilityResult, err := s.ensureAvailability(c, 3, constraints.MustParse("mem=4G"), defaultSeries, nil)
	c.Assert(err, gc.IsNil)
	c.Assert(ensureAvailabilityResult.Maintained, gc.DeepEquals, []string{"machine-0"})
	c.Assert(ensureAvailabilityResult.Added, gc.DeepEquals, []string{"machine-1", "machine-2"})
	c.Assert(ensureAvailabilityResult.Removed, gc.HasLen, 0)

	machines, err := s.State.AllMachines()
	c.Assert(err, gc.IsNil)
	c.Assert(machines, gc.HasLen, 3)
	expectedCons := []constraints.Value{
		constraints.Value{},
		constraints.MustParse("mem=4G"),
		constraints.MustParse("mem=4G"),
	}
	for i, m := range machines {
		cons, err := m.Constraints()
		c.Assert(err, gc.IsNil)
		c.Check(cons, gc.DeepEquals, expectedCons[i])
	}
}

func (s *clientSuite) TestEnsureAvailabilityPlacement(c *gc.C) {
	placement := []string{"valid"}
	ensureAvailabilityResult, err := s.ensureAvailability(c, 3, constraints.MustParse("mem=4G"), defaultSeries, placement)
	c.Assert(err, gc.IsNil)
	c.Assert(ensureAvailabilityResult.Maintained, gc.DeepEquals, []string{"machine-0"})
	c.Assert(ensureAvailabilityResult.Added, gc.DeepEquals, []string{"machine-1", "machine-2"})
	c.Assert(ensureAvailabilityResult.Removed, gc.HasLen, 0)

	machines, err := s.State.AllMachines()
	c.Assert(err, gc.IsNil)
	c.Assert(machines, gc.HasLen, 3)
	expectedCons := []constraints.Value{
		constraints.Value{},
		constraints.MustParse("mem=4G"),
		constraints.MustParse("mem=4G"),
	}
	expectedPlacement := []string{"", "valid", ""}
	for i, m := range machines {
		cons, err := m.Constraints()
		c.Assert(err, gc.IsNil)
		c.Check(cons, gc.DeepEquals, expectedCons[i])
		c.Check(m.Placement(), gc.Equals, expectedPlacement[i])
	}
}

func (s *clientSuite) TestEnsureAvailability0Preserves(c *gc.C) {
	// A value of 0 says either "if I'm not HA, make me HA" or "preserve my
	// current HA settings".
	ensureAvailabilityResult, err := s.ensureAvailability(c, 0, emptyCons, defaultSeries, nil)
	c.Assert(err, gc.IsNil)
	c.Assert(ensureAvailabilityResult.Maintained, gc.DeepEquals, []string{"machine-0"})
	c.Assert(ensureAvailabilityResult.Added, gc.DeepEquals, []string{"machine-1", "machine-2"})
	c.Assert(ensureAvailabilityResult.Removed, gc.HasLen, 0)

	machines, err := s.State.AllMachines()
	c.Assert(machines, gc.HasLen, 3)

	pingerB := s.setAgentPresence(c, "1")
	defer assertKill(c, pingerB)

	// Now, we keep agent 1 alive, but not agent 2, calling
	// EnsureAvailability(0) again will cause us to start another machine
	ensureAvailabilityResult, err = s.ensureAvailability(c, 0, emptyCons, defaultSeries, nil)
	c.Assert(err, gc.IsNil)
	c.Assert(ensureAvailabilityResult.Maintained, gc.DeepEquals, []string{"machine-0", "machine-1"})
	c.Assert(ensureAvailabilityResult.Added, gc.DeepEquals, []string{"machine-3"})
	c.Assert(ensureAvailabilityResult.Removed, gc.HasLen, 0)

	machines, err = s.State.AllMachines()
	c.Assert(machines, gc.HasLen, 4)
}

func (s *clientSuite) TestEnsureAvailability0Preserves5(c *gc.C) {
	// Start off with 5 servers
	ensureAvailabilityResult, err := s.ensureAvailability(c, 5, emptyCons, defaultSeries, nil)
	c.Assert(err, gc.IsNil)
	c.Assert(ensureAvailabilityResult.Maintained, gc.DeepEquals, []string{"machine-0"})
	c.Assert(ensureAvailabilityResult.Added, gc.DeepEquals, []string{"machine-1", "machine-2", "machine-3", "machine-4"})
	c.Assert(ensureAvailabilityResult.Removed, gc.HasLen, 0)

	machines, err := s.State.AllMachines()
	c.Assert(machines, gc.HasLen, 5)
	pingerB := s.setAgentPresence(c, "1")
	defer assertKill(c, pingerB)

	pingerC := s.setAgentPresence(c, "2")
	defer assertKill(c, pingerC)

	pingerD := s.setAgentPresence(c, "3")
	defer assertKill(c, pingerD)
	// Keeping all alive but one, will bring up 1 more server to preserve 5
	ensureAvailabilityResult, err = s.ensureAvailability(c, 0, emptyCons, defaultSeries, nil)
	c.Assert(err, gc.IsNil)
	c.Assert(ensureAvailabilityResult.Maintained, gc.DeepEquals, []string{"machine-0", "machine-1",
		"machine-2", "machine-3"})
	c.Assert(ensureAvailabilityResult.Added, gc.DeepEquals, []string{"machine-5"})
	c.Assert(ensureAvailabilityResult.Removed, gc.HasLen, 0)

	machines, err = s.State.AllMachines()
	c.Assert(machines, gc.HasLen, 6)
	c.Assert(err, gc.IsNil)
}

func (s *clientSuite) TestEnsureAvailabilityErrors(c *gc.C) {
	ensureAvailabilityResult, err := s.ensureAvailability(c, -1, emptyCons, defaultSeries, nil)
	c.Assert(err, gc.ErrorMatches, "number of state servers must be odd and non-negative")

	ensureAvailabilityResult, err = s.ensureAvailability(c, 3, emptyCons, defaultSeries, nil)
	c.Assert(err, gc.IsNil)
	c.Assert(ensureAvailabilityResult.Maintained, gc.DeepEquals, []string{"machine-0"})
	c.Assert(ensureAvailabilityResult.Added, gc.DeepEquals, []string{"machine-1", "machine-2"})
	c.Assert(ensureAvailabilityResult.Removed, gc.HasLen, 0)

	_, err = s.ensureAvailability(c, 1, emptyCons, defaultSeries, nil)
	c.Assert(err, gc.ErrorMatches, "failed to create new state server machines: cannot reduce state server count")
}
