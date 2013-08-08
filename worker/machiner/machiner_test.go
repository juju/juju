// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machiner_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	apimachiner "launchpad.net/juju-core/state/api/machiner"
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

	st            *api.State
	machinerState *apimachiner.State
	machine       *state.Machine
	apiMachine    *apimachiner.Machine
}

var _ = Suite(&MachinerSuite{})

func (s *MachinerSuite) SetUpTest(c *C) {
	s.JujuConnSuite.SetUpTest(c)

	// Create a machine so we can log in as its agent.
	var err error
	s.machine, err = s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	err = s.machine.SetProvisioned("foo", "fake_nonce", nil)
	c.Assert(err, IsNil)
	err = s.machine.SetPassword("password")
	c.Assert(err, IsNil)
	s.st = s.OpenAPIAsMachine(c, s.machine.Tag(), "password", "fake_nonce")

	// Create the machiner API facade.
	s.machinerState = s.st.Machiner()
	c.Assert(s.machinerState, NotNil)

	// Get the machine through the facade.
	s.apiMachine, err = s.machinerState.Machine(s.machine.Tag())
	c.Assert(err, IsNil)
	c.Assert(s.apiMachine.Tag(), Equals, s.machine.Tag())
}

func (s *MachinerSuite) TearDownTest(c *C) {
	err := s.st.Close()
	c.Assert(err, IsNil)
	s.JujuConnSuite.TearDownTest(c)
}

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

var _ worker.NotifyWatchHandler = (*machiner.Machiner)(nil)

func (s *MachinerSuite) TestNotFoundOrUnauthorized(c *C) {
	mr := machiner.NewMachiner(s.machinerState, "eleventy-one")
	c.Assert(mr.Wait(), Equals, worker.ErrTerminateAgent)
}

func (s *MachinerSuite) TestRunStop(c *C) {
	mr := machiner.NewMachiner(s.machinerState, s.apiMachine.Tag())
	c.Assert(worker.Stop(mr), IsNil)
	c.Assert(s.apiMachine.Refresh(), IsNil)
	c.Assert(s.apiMachine.Life(), Equals, params.Alive)
}

func (s *MachinerSuite) TestStartSetsStatus(c *C) {
	status, info, err := s.machine.Status()
	c.Assert(err, IsNil)
	c.Assert(status, Equals, params.StatusPending)
	c.Assert(info, Equals, "")

	mr := machiner.NewMachiner(s.machinerState, s.apiMachine.Tag())
	defer worker.Stop(mr)

	s.waitMachineStatus(c, s.machine, params.StatusStarted)
}

func (s *MachinerSuite) TestSetsStatusWhenDying(c *C) {
	mr := machiner.NewMachiner(s.machinerState, s.apiMachine.Tag())
	defer worker.Stop(mr)
	c.Assert(s.machine.Destroy(), IsNil)
	s.waitMachineStatus(c, s.machine, params.StatusStopped)
}

func (s *MachinerSuite) TestSetDead(c *C) {
	mr := machiner.NewMachiner(s.machinerState, s.machine.Tag())
	defer worker.Stop(mr)
	c.Assert(s.machine.Destroy(), IsNil)
	s.State.StartSync()
	c.Assert(mr.Wait(), Equals, worker.ErrTerminateAgent)
	c.Assert(s.machine.Refresh(), IsNil)
	c.Assert(s.machine.Life(), Equals, state.Dead)
}
