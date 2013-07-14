// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"time"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/container/lxc"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/testing"
)

var _ = gc.Suite(&UpgradeValidationSuite{})

type UpgradeValidationSuite struct {
	agentSuite
	lxc.TestSuite
}

func (s *UpgradeValidationSuite) SetUpSuite(c *gc.C) {
	s.agentSuite.SetUpSuite(c)
	s.TestSuite.SetUpSuite(c)
}

func (s *UpgradeValidationSuite) TearDownSuite(c *gc.C) {
	s.TestSuite.TearDownSuite(c)
	s.agentSuite.TearDownSuite(c)
}

func (s *UpgradeValidationSuite) SetUpTest(c *gc.C) {
	s.agentSuite.SetUpTest(c)
	s.TestSuite.SetUpTest(c)
}

func (s *UpgradeValidationSuite) TearDownTest(c *gc.C) {
	s.TestSuite.TearDownTest(c)
	s.agentSuite.TearDownTest(c)
}

func (s *UpgradeValidationSuite) Create1_10Machine(c *gc.C) (*state.Machine, *agent.Conf) {
	// Given the current connection to state, create a new machine, and 'reset'
	// the configuration so that it looks like how juju 1.10 would have
	// configured it
	m, err := s.State.InjectMachine("series", constraints.Value{}, "ardbeg-0", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	err = m.SetMongoPassword("machine-password")
	c.Assert(err, gc.IsNil)
	// We intentionally do *not* call m.SetPassword here, as it was not
	// done in 1.10, we also intentionally set the APIInfo.Password back to
	// the empty string.
	conf, _ := s.agentSuite.primeAgent(c, m.Tag(), "machine-password")
	conf.MachineNonce = state.BootstrapNonce
	conf.APIInfo.Password = ""
	conf.Write()
	c.Assert(err, gc.IsNil)
	return m, conf
}

func (s *UpgradeValidationSuite) TestEnsureAPIPasswordMachine(c *gc.C) {
	m, conf := s.Create1_10Machine(c)
	// Opening the API should fail as is
	apiState, newPassword, err := conf.OpenAPI(api.DialOpts{})
	c.Assert(apiState, gc.IsNil)
	c.Assert(newPassword, gc.Equals, "")
	c.Assert(err, gc.NotNil)
	c.Assert(err, gc.ErrorMatches, "invalid entity name or password")

	err = EnsureAPIPassword(conf, m)
	c.Assert(err, gc.IsNil)
	// After EnsureAPIPassword we should be able to connect
	apiState, newPassword, err = conf.OpenAPI(api.DialOpts{})
	c.Assert(err, gc.IsNil)
	c.Assert(apiState, gc.NotNil)
	// We shouldn't need to set a new password
	c.Assert(newPassword, gc.Equals, "")
}

// Test that MachineAgent enforces the API password on startup
func (s *UpgradeValidationSuite) TestMachineAgentEnsuresAPIPassword(c *gc.C) {
	m, _ := s.Create1_10Machine(c)
	// This is similar to assertJobWithState, however we need to control
	// how the machine is initialized, so it looks like a 1.10 upgrade
	a := &MachineAgent{}
	s.initAgent(c, a, "--machine-id", m.Id())

	agentStates := make(chan *state.State, 1000)
	undo := sendOpenedStates(agentStates)
	defer undo()

	done := make(chan error)
	go func() {
		done <- a.Run(nil)
	}()

	select {
	case agentState := <-agentStates:
		c.Assert(agentState, gc.NotNil)
		c.Assert(a.Conf.Conf.APIInfo.Password, gc.Equals, "machine-password")
	case <-time.After(testing.LongWait):
		c.Fatalf("state not opened")
	}
	err := a.Stop()
	c.Assert(err, gc.IsNil)
	c.Assert(<-done, gc.IsNil)
}

// Test that MachineAgent enforces the API password on startup even for machine>0
func (s *UpgradeValidationSuite) TestMachineAgentEnsuresAPIPasswordOnWorkers(c *gc.C) {
	// create a machine-0, then create a new machine-1
	_, _ = s.Create1_10Machine(c)
	m1, _ := s.Create1_10Machine(c)

	a := &MachineAgent{}
	s.initAgent(c, a, "--machine-id", m1.Id())

	agentStates := make(chan *state.State, 1000)
	undo := sendOpenedStates(agentStates)
	defer undo()

	done := make(chan error)
	go func() {
		done <- a.Run(nil)
	}()

	select {
	case agentState := <-agentStates:
		c.Assert(agentState, gc.NotNil)
		c.Assert(a.Conf.Conf.APIInfo.Password, gc.Equals, "machine-password")
	case <-time.After(testing.LongWait):
		c.Fatalf("state not opened")
	}
	err := a.Stop()
	c.Assert(err, gc.IsNil)
	c.Assert(<-done, gc.IsNil)
}

func (s *UpgradeValidationSuite) TestEnsureAPIPasswordMachineNoOp(c *gc.C) {
	m, conf := s.Create1_10Machine(c)
	// Set the API password to something, and record it, ensure that
	// EnsureAPIPassword doesn't change it on us
	m.SetPassword("frobnizzle")
	conf.APIInfo.Password = "frobnizzle"
	// We matched them, so we should be able to open the API
	apiState, newPassword, err := conf.OpenAPI(api.DialOpts{})
	c.Assert(apiState, gc.NotNil)
	c.Assert(newPassword, gc.Equals, "")
	c.Assert(err, gc.IsNil)
	apiState.Close()

	err = EnsureAPIPassword(conf, m)
	c.Assert(err, gc.IsNil)
	// After EnsureAPIPassword we should still be able to connect
	apiState, newPassword, err = conf.OpenAPI(api.DialOpts{})
	c.Assert(err, gc.IsNil)
	c.Assert(apiState, gc.NotNil)
	// We shouldn't need to set a new password
	c.Assert(newPassword, gc.Equals, "")
	// The password hasn't been changed
	c.Assert(conf.APIInfo.Password, gc.Equals, "frobnizzle")
}

func (s *UpgradeValidationSuite) Create1_10Unit(c *gc.C) (*state.Unit, *agent.Conf) {
	svc, err := s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, gc.IsNil)
	unit, err := svc.AddUnit()
	c.Assert(err, gc.IsNil)
	err = unit.SetMongoPassword("unit-password")
	c.Assert(err, gc.IsNil)
	// We do not call SetPassword for the unit agent, and we force the
	// APIInfo.Password to be empty
	conf, _ := s.agentSuite.primeAgent(c, unit.Tag(), "unit-password")
	conf.APIInfo.Password = ""
	c.Assert(conf.Write(), gc.IsNil)
	return unit, conf
}

func (s *UpgradeValidationSuite) TestEnsureAPIPasswordUnit(c *gc.C) {
	u, conf := s.Create1_10Unit(c)
	// Opening the API should fail as is
	apiState, newPassword, err := conf.OpenAPI(api.DialOpts{})
	c.Assert(apiState, gc.IsNil)
	c.Assert(newPassword, gc.Equals, "")
	c.Assert(err, gc.NotNil)
	c.Assert(err, gc.ErrorMatches, "invalid entity name or password")

	err = EnsureAPIPassword(conf, u)
	c.Assert(err, gc.IsNil)
	// After EnsureAPIPassword we should be able to connect
	apiState, newPassword, err = conf.OpenAPI(api.DialOpts{})
	c.Assert(err, gc.IsNil)
	c.Assert(apiState, gc.NotNil)
	// We shouldn't need to set a new password
	c.Assert(newPassword, gc.Equals, "")
}
