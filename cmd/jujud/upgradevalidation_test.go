// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"time"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/container/lxc"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/testing"
)

var _ = gc.Suite(&UpgradeValidationMachineSuite{})

type UpgradeValidationMachineSuite struct {
	agentSuite
	lxc.TestSuite
}

func (s *UpgradeValidationMachineSuite) SetUpSuite(c *gc.C) {
	s.agentSuite.SetUpSuite(c)
	s.TestSuite.SetUpSuite(c)
}

func (s *UpgradeValidationMachineSuite) TearDownSuite(c *gc.C) {
	s.TestSuite.TearDownSuite(c)
	s.agentSuite.TearDownSuite(c)
}

func (s *UpgradeValidationMachineSuite) SetUpTest(c *gc.C) {
	s.agentSuite.SetUpTest(c)
	s.TestSuite.SetUpTest(c)
}

func (s *UpgradeValidationMachineSuite) TearDownTest(c *gc.C) {
	s.TestSuite.TearDownTest(c)
	s.agentSuite.TearDownTest(c)
}

func (s *UpgradeValidationMachineSuite) Create1_10Machine(c *gc.C) (*state.Machine, *agent.Conf) {
	// Given the current connection to state, create a new machine, and 'reset'
	// the configuration so that it looks like how juju 1.10 would have
	// configured it
	m, err := s.State.InjectMachine("series", constraints.Value{}, "ardbeg-0", instance.HardwareCharacteristics{}, state.JobHostUnits)
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
	c.Assert(conf.StateInfo.Tag, gc.Equals, m.Tag())
	c.Assert(conf.StateInfo.Password, gc.Equals, "machine-password")
	c.Assert(err, gc.IsNil)
	return m, conf
}

func (s *UpgradeValidationMachineSuite) TestEnsureAPIInfo(c *gc.C) {
	m, conf := s.Create1_10Machine(c)
	// Opening the API should fail as is
	apiState, newPassword, err := conf.OpenAPI(api.DialOpts{})
	c.Assert(apiState, gc.IsNil)
	c.Assert(newPassword, gc.Equals, "")
	c.Assert(err, gc.NotNil)
	c.Assert(err, gc.ErrorMatches, "invalid entity name or password")

	err = EnsureAPIInfo(conf, s.State, m)
	c.Assert(err, gc.IsNil)
	// After EnsureAPIInfo we should be able to connect
	apiState, newPassword, err = conf.OpenAPI(api.DialOpts{})
	c.Assert(err, gc.IsNil)
	c.Assert(apiState, gc.NotNil)
	// We shouldn't need to set a new password
	c.Assert(newPassword, gc.Equals, "")
}

func (s *UpgradeValidationMachineSuite) TestEnsureAPIInfoNoOp(c *gc.C) {
	m, conf := s.Create1_10Machine(c)
	// Set the API password to something, and record it, ensure that
	// EnsureAPIInfo doesn't change it on us
	m.SetPassword("frobnizzle")
	conf.APIInfo.Password = "frobnizzle"
	// We matched them, so we should be able to open the API
	apiState, newPassword, err := conf.OpenAPI(api.DialOpts{})
	c.Assert(apiState, gc.NotNil)
	c.Assert(newPassword, gc.Equals, "")
	c.Assert(err, gc.IsNil)
	apiState.Close()

	err = EnsureAPIInfo(conf, s.State, m)
	c.Assert(err, gc.IsNil)
	// After EnsureAPIInfo we should still be able to connect
	apiState, newPassword, err = conf.OpenAPI(api.DialOpts{})
	c.Assert(err, gc.IsNil)
	c.Assert(apiState, gc.NotNil)
	// We shouldn't need to set a new password
	c.Assert(newPassword, gc.Equals, "")
	// The password hasn't been changed
	c.Assert(conf.APIInfo.Password, gc.Equals, "frobnizzle")
}

// Test that MachineAgent enforces the API password on startup
func (s *UpgradeValidationMachineSuite) TestAgentEnsuresAPIInfo(c *gc.C) {
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
func (s *UpgradeValidationMachineSuite) TestAgentEnsuresAPIInfoOnWorkers(c *gc.C) {
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

var _ = gc.Suite(&UpgradeValidationUnitSuite{})

type UpgradeValidationUnitSuite struct {
	agentSuite
	testing.GitSuite
}

func (s *UpgradeValidationUnitSuite) SetUpTest(c *gc.C) {
	s.agentSuite.SetUpTest(c)
	s.GitSuite.SetUpTest(c)
}

func (s *UpgradeValidationUnitSuite) TearDownTest(c *gc.C) {
	s.GitSuite.SetUpTest(c)
	s.agentSuite.TearDownTest(c)
}

func (s *UpgradeValidationUnitSuite) Create1_10Unit(c *gc.C) (*state.Unit, *agent.Conf) {
	svc, err := s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, gc.IsNil)
	unit, err := svc.AddUnit()
	c.Assert(err, gc.IsNil)
	err = unit.SetMongoPassword("unit-password")
	c.Assert(err, gc.IsNil)
	// We do not call SetPassword for the unit agent, and we force the
	// APIInfo to be empty
	conf, _ := s.agentSuite.primeAgent(c, unit.Tag(), "unit-password")
	conf.APIInfo = nil
	c.Assert(conf.Write(), gc.IsNil)
	return unit, conf
}

func (s *UpgradeValidationUnitSuite) TestEnsureAPIInfo(c *gc.C) {
	u, conf := s.Create1_10Unit(c)
	// Opening the API should fail as is
	c.Assert(func() { conf.OpenAPI(api.DialOpts{}) }, gc.PanicMatches, ".*nil pointer dereference")

	err := EnsureAPIInfo(conf, s.State, u)
	c.Assert(err, gc.IsNil)
	// The test suite runs the API on non-standard ports. Fix it
	apiAddresses, err := s.State.APIAddresses()
	c.Assert(err, gc.IsNil)
	c.Assert(conf.APIInfo.Addrs, gc.DeepEquals, apiAddresses)
	conf.APIInfo.Addrs = s.APIInfo(c).Addrs
	apiState, newPassword, err := conf.OpenAPI(api.DialOpts{})
	c.Assert(err, gc.IsNil)
	c.Assert(apiState, gc.NotNil)
	// We shouldn't need to set a new password
	c.Assert(newPassword, gc.Equals, "")
}

func (s *UpgradeValidationUnitSuite) TestEnsureAPIInfo1_11(c *gc.C) {
	// In 1.11 State.Password is actually "", and the valid password is
	// OldPassword. This is because in 1.11 we only change the password in
	// OpenAPI which we don't call until we actually have agent workers
	// But we don't want to set the actual entity password to the empty string
	u, conf := s.Create1_10Unit(c)
	conf.OldPassword = conf.StateInfo.Password
	conf.StateInfo.Password = ""

	err := EnsureAPIInfo(conf, s.State, u)
	c.Assert(err, gc.IsNil)
	// The test suite runs the API on non-standard ports. Fix it
	apiAddresses, err := s.State.APIAddresses()
	c.Assert(err, gc.IsNil)
	c.Assert(conf.APIInfo.Addrs, gc.DeepEquals, apiAddresses)
	conf.APIInfo.Addrs = s.APIInfo(c).Addrs
	apiState, newPassword, err := conf.OpenAPI(api.DialOpts{})
	c.Assert(err, gc.IsNil)
	c.Assert(apiState, gc.NotNil)
	// It should want to set a new Password
	c.Assert(newPassword, gc.Not(gc.Equals), "")
}

func (s *UpgradeValidationUnitSuite) TestEnsureAPIInfo1_11Noop(c *gc.C) {
	// We should notice if APIInfo is 'valid' in that it matches StateInfo
	// even though in 1.1 both Password fields are empty.
	u, conf := s.Create1_10Unit(c)
	conf.OldPassword = conf.StateInfo.Password
	conf.StateInfo.Password = ""
	u.SetPassword(conf.OldPassword)
	testAPIInfo := s.APIInfo(c)
	conf.APIInfo = &api.Info{
		Addrs:    testAPIInfo.Addrs,
		Tag:      u.Tag(),
		Password: "",
		CACert:   testAPIInfo.CACert,
	}

	err := EnsureAPIInfo(conf, s.State, u)
	c.Assert(err, gc.IsNil)
	// We should not have changed the API Addrs or Password
	c.Assert(conf.APIInfo.Password, gc.Equals, "")
	c.Assert(conf.APIInfo.Addrs, gc.DeepEquals, testAPIInfo.Addrs)
	apiState, newPassword, err := conf.OpenAPI(api.DialOpts{})
	c.Assert(err, gc.IsNil)
	c.Assert(apiState, gc.NotNil)
	// It should want to set a new Password
	c.Assert(newPassword, gc.Not(gc.Equals), "")
}

// Test that UnitAgent enforces the API password on startup
func (s *UpgradeValidationUnitSuite) TestAgentEnsuresAPIInfo(c *gc.C) {
	unit, _ := s.Create1_10Unit(c)
	a := &UnitAgent{}
	s.initAgent(c, a, "--unit-name", unit.Name())
	go func() { c.Check(a.Run(nil), gc.IsNil) }()
	waitForUnitStarted(s.State, unit, c)
	c.Check(a.Stop(), gc.IsNil)
	c.Check(a.Conf.APIInfo.Password, gc.Equals, "unit-password")
}

var _ = gc.Suite(&UpgradeValidationSuite{})

type UpgradeValidationSuite struct {
	testing.LoggingSuite
}

type mockAddresser struct {
	Addrs []string
	Err   error
}

func (m *mockAddresser) APIAddresses() ([]string, error) {
	return m.Addrs, m.Err
}

func (s *UpgradeValidationSuite) TestapiInfoFromStateInfo(c *gc.C) {
	cert := []byte("stuff")
	stInfo := &state.Info{
		Addrs:    []string{"example.invalid:37070"},
		CACert:   cert,
		Tag:      "machine-0",
		Password: "abcdefh",
	}
	apiAddresses := []string{"example.invalid:17070", "another.invalid:1234"}
	apiInfo := apiInfoFromStateInfo(stInfo, &mockAddresser{Addrs: apiAddresses})
	c.Assert(*apiInfo, gc.DeepEquals, api.Info{
		Addrs:    apiAddresses,
		CACert:   cert,
		Tag:      "machine-0",
		Password: "abcdefh",
	})

}

func (s *UpgradeValidationSuite) TestapiInfoFromStateInfoSwallowsError(c *gc.C) {
	// No reason for it to die just because of this
	cert := []byte("stuff")
	stInfo := &state.Info{
		Addrs:    []string{"example.invalid:37070"},
		CACert:   cert,
		Tag:      "machine-0",
		Password: "abcdefh",
	}
	apiAddresses := []string{}
	apiInfo := apiInfoFromStateInfo(stInfo, &mockAddresser{Addrs: apiAddresses, Err: fmt.Errorf("bad")})
	c.Assert(*apiInfo, gc.DeepEquals, api.Info{
		Addrs:    []string{},
		CACert:   cert,
		Tag:      "machine-0",
		Password: "abcdefh",
	})

}
