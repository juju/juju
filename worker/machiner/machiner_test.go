// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machiner_test

import (
	gc "launchpad.net/gocheck"
	stdtesting "testing"
	"time"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	apimachiner "launchpad.net/juju-core/state/api/machiner"
	"launchpad.net/juju-core/state/api/params"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/worker"
	"launchpad.net/juju-core/worker/machiner"
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

var _ = gc.Suite(&MachinerSuite{})

func (s *MachinerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.st, s.machine = s.OpenAPIAsNewMachine(c)

	// Create the machiner API facade.
	s.machinerState = s.st.Machiner()
	c.Assert(s.machinerState, gc.NotNil)

	// Get the machine through the facade.
	var err error
	s.apiMachine, err = s.machinerState.Machine(s.machine.Tag())
	c.Assert(err, gc.IsNil)
	c.Assert(s.apiMachine.Tag(), gc.Equals, s.machine.Tag())
}

func (s *MachinerSuite) TearDownTest(c *gc.C) {
	err := s.st.Close()
	c.Assert(err, gc.IsNil)
	s.JujuConnSuite.TearDownTest(c)
}

func (s *MachinerSuite) waitMachineStatus(c *gc.C, m *state.Machine, expectStatus params.Status) {
	timeout := time.After(worstCase)
	for {
		select {
		case <-timeout:
			c.Fatalf("timeout while waiting for machine status to change")
		case <-time.After(10 * time.Millisecond):
			status, _, err := m.Status()
			c.Assert(err, gc.IsNil)
			if status != expectStatus {
				c.Logf("machine %q status is %s, still waiting", m, status)
				continue
			}
			return
		}
	}
}

var _ worker.NotifyWatchHandler = (*machiner.Machiner)(nil)

type mockConfig struct {
	agent.Config
	tag string
}

func (mock *mockConfig) Tag() string {
	return mock.tag
}

func agentConfig(tag string) agent.Config {
	return &mockConfig{tag: tag}
}

func (s *MachinerSuite) TestNotFoundOrUnauthorized(c *gc.C) {
	mr := machiner.NewMachiner(s.machinerState, agentConfig("eleventy-one"))
	c.Assert(mr.Wait(), gc.Equals, worker.ErrTerminateAgent)
}

func (s *MachinerSuite) makeMachiner() worker.Worker {
	return machiner.NewMachiner(s.machinerState, agentConfig(s.apiMachine.Tag()))
}

func (s *MachinerSuite) TestRunStop(c *gc.C) {
	mr := s.makeMachiner()
	c.Assert(worker.Stop(mr), gc.IsNil)
	c.Assert(s.apiMachine.Refresh(), gc.IsNil)
	c.Assert(s.apiMachine.Life(), gc.Equals, params.Alive)
}

func (s *MachinerSuite) TestStartSetsStatus(c *gc.C) {
	status, info, err := s.machine.Status()
	c.Assert(err, gc.IsNil)
	c.Assert(status, gc.Equals, params.StatusPending)
	c.Assert(info, gc.Equals, "")

	mr := s.makeMachiner()
	defer worker.Stop(mr)

	s.waitMachineStatus(c, s.machine, params.StatusStarted)
}

func (s *MachinerSuite) TestSetsStatusWhenDying(c *gc.C) {
	mr := s.makeMachiner()
	defer worker.Stop(mr)
	c.Assert(s.machine.Destroy(), gc.IsNil)
	s.waitMachineStatus(c, s.machine, params.StatusStopped)
}

func (s *MachinerSuite) TestSetDead(c *gc.C) {
	mr := s.makeMachiner()
	defer worker.Stop(mr)
	c.Assert(s.machine.Destroy(), gc.IsNil)
	s.State.StartSync()
	c.Assert(mr.Wait(), gc.Equals, worker.ErrTerminateAgent)
	c.Assert(s.machine.Refresh(), gc.IsNil)
	c.Assert(s.machine.Life(), gc.Equals, state.Dead)
}
