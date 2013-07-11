// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machiner_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/worker"
	"launchpad.net/juju-core/worker/machiner"
	stdtesting "testing"
	"time"
)

// worstCase is used for timeouts when timing out
// will fail the test. Raising this value should
// not affect the overall running time of the tests
// unless they fail.
const worstCase = 5 * time.Second

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type MachinerSuite struct {
	testing.JujuConnSuite
}

var _ = Suite(&MachinerSuite{})

func (s *MachinerSuite) waitMachineStatus(c *C, m *state.Machine, expectStatus params.Status) {
	timeout := time.After(worstCase)
	for {
		select {
		case <-timeout:
			c.Fatalf("timeout while waiting for machine status to change")
		case <-time.After(10 * time.Millisecond):
			status, _, err := m.Status()
			c.Assert(err, IsNil)
			if status != expectStatus {
				c.Logf("machine %q status is %s, still waiting", m, status)
				continue
			}
			return
		}
	}
}

var _ worker.WatchHandler = (*machiner.Machiner)(nil)

func (s *MachinerSuite) TestNotFound(c *C) {
	mr := machiner.NewMachiner(s.State, "eleventy-one")
	c.Assert(mr.Wait(), Equals, worker.ErrTerminateAgent)
}

func (s *MachinerSuite) TestRunStop(c *C) {
	m, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)

	mr := machiner.NewMachiner(s.State, m.Id())

	c.Assert(mr.Stop(), IsNil)
	c.Assert(m.Refresh(), IsNil)
	c.Assert(m.Life(), Equals, state.Alive)
}

func (s *MachinerSuite) TestStartSetsStatus(c *C) {
	m, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)

	status, info, err := m.Status()
	c.Assert(err, IsNil)
	c.Assert(status, Equals, params.StatusPending)
	c.Assert(info, Equals, "")

	alive, err := m.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, false)

	mr := machiner.NewMachiner(s.State, m.Id())
	defer mr.Stop()

	s.waitMachineStatus(c, m, params.StatusStarted)

	s.State.Sync()
	alive, err = m.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, true)
}

func (s *MachinerSuite) TestSetsStatusWhenDying(c *C) {
	m, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	mr := machiner.NewMachiner(s.State, m.Id())
	defer mr.Stop()
	c.Assert(m.Destroy(), IsNil)
	s.waitMachineStatus(c, m, params.StatusStopped)
}

func (s *MachinerSuite) TestSetDead(c *C) {
	m, err := s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	mr := machiner.NewMachiner(s.State, m.Id())
	defer mr.Stop()
	c.Assert(m.Destroy(), IsNil)
	s.State.StartSync()
	c.Assert(mr.Wait(), Equals, worker.ErrTerminateAgent)
	c.Assert(m.Refresh(), IsNil)
	c.Assert(m.Life(), Equals, state.Dead)
}
