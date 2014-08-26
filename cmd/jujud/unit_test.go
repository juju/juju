// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"reflect"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/names"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/agent"
	envtesting "github.com/juju/juju/environs/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/api/params"
	apirsyslog "github.com/juju/juju/state/api/rsyslog"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/rsyslog"
	"github.com/juju/juju/worker/upgrader"
)

type UnitSuite struct {
	coretesting.GitSuite
	agentSuite
}

var _ = gc.Suite(&UnitSuite{})

func (s *UnitSuite) SetUpTest(c *gc.C) {
	s.GitSuite.SetUpTest(c)
	s.agentSuite.SetUpTest(c)
}

func (s *UnitSuite) TearDownTest(c *gc.C) {
	s.agentSuite.TearDownTest(c)
	s.GitSuite.TearDownTest(c)
}

const initialUnitPassword = "unit-password-1234567890"

// primeAgent creates a unit, and sets up the unit agent's directory.
// It returns the assigned machine, new unit and the agent's configuration.
func (s *UnitSuite) primeAgent(c *gc.C) (*state.Machine, *state.Unit, agent.Config, *tools.Tools) {
	jujutesting.AddStateServerMachine(c, s.State)
	svc := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	unit, err := svc.AddUnit()
	c.Assert(err, gc.IsNil)
	err = unit.SetPassword(initialUnitPassword)
	c.Assert(err, gc.IsNil)
	// Assign the unit to a machine.
	err = unit.AssignToNewMachine()
	c.Assert(err, gc.IsNil)
	id, err := unit.AssignedMachineId()
	c.Assert(err, gc.IsNil)
	machine, err := s.State.Machine(id)
	c.Assert(err, gc.IsNil)
	conf, tools := s.agentSuite.primeAgent(c, unit.Tag().String(), initialUnitPassword, version.Current)
	return machine, unit, conf, tools
}

func (s *UnitSuite) newAgent(c *gc.C, unit *state.Unit) *UnitAgent {
	a := &UnitAgent{}
	s.initAgent(c, a, "--unit-name", unit.Name())
	err := a.ReadConfig(unit.Tag().String())
	c.Assert(err, gc.IsNil)
	return a
}

func (s *UnitSuite) TestParseSuccess(c *gc.C) {
	create := func() (cmd.Command, *AgentConf) {
		a := &UnitAgent{}
		return a, &a.AgentConf
	}
	uc := CheckAgentCommand(c, create, []string{"--unit-name", "w0rd-pre55/1"})
	c.Assert(uc.(*UnitAgent).UnitName, gc.Equals, "w0rd-pre55/1")
}

func (s *UnitSuite) TestParseMissing(c *gc.C) {
	uc := &UnitAgent{}
	err := ParseAgentCommand(uc, []string{})
	c.Assert(err, gc.ErrorMatches, "--unit-name option must be set")
}

func (s *UnitSuite) TestParseNonsense(c *gc.C) {
	for _, args := range [][]string{
		{"--unit-name", "wordpress"},
		{"--unit-name", "wordpress/seventeen"},
		{"--unit-name", "wordpress/-32"},
		{"--unit-name", "wordpress/wild/9"},
		{"--unit-name", "20/20"},
	} {
		err := ParseAgentCommand(&UnitAgent{}, args)
		c.Assert(err, gc.ErrorMatches, `--unit-name option expects "<service>/<n>" argument`)
	}
}

func (s *UnitSuite) TestParseUnknown(c *gc.C) {
	uc := &UnitAgent{}
	err := ParseAgentCommand(uc, []string{"--unit-name", "wordpress/1", "thundering typhoons"})
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["thundering typhoons"\]`)
}

func waitForUnitStarted(stateConn *state.State, unit *state.Unit, c *gc.C) {
	timeout := time.After(5 * time.Second)

	for {
		select {
		case <-timeout:
			c.Fatalf("no activity detected")
		case <-time.After(coretesting.ShortWait):
			err := unit.Refresh()
			c.Assert(err, gc.IsNil)
			st, info, data, err := unit.Status()
			c.Assert(err, gc.IsNil)
			switch st {
			case params.StatusPending, params.StatusInstalled:
				c.Logf("waiting...")
				continue
			case params.StatusStarted:
				c.Logf("started!")
				return
			case params.StatusDown:
				stateConn.StartSync()
				c.Logf("unit is still down")
			default:
				c.Fatalf("unexpected status %s %s %v", st, info, data)
			}
		}
	}
}

func (s *UnitSuite) TestRunStop(c *gc.C) {
	_, unit, _, _ := s.primeAgent(c)
	a := s.newAgent(c, unit)
	go func() { c.Check(a.Run(nil), gc.IsNil) }()
	defer func() { c.Check(a.Stop(), gc.IsNil) }()
	waitForUnitStarted(s.State, unit, c)
}

func (s *UnitSuite) TestUpgrade(c *gc.C) {
	machine, unit, _, currentTools := s.primeAgent(c)
	agent := s.newAgent(c, unit)
	newVers := version.Current
	newVers.Patch++
	envtesting.AssertUploadFakeToolsVersions(c, s.Conn.Environ.Storage(), newVers)

	// Set the machine agent version to trigger an upgrade.
	err := machine.SetAgentVersion(newVers)
	c.Assert(err, gc.IsNil)
	err = runWithTimeout(agent)
	envtesting.CheckUpgraderReadyError(c, err, &upgrader.UpgradeReadyError{
		AgentName: unit.Tag().String(),
		OldTools:  currentTools.Version,
		NewTools:  newVers,
		DataDir:   s.DataDir(),
	})
}

func (s *UnitSuite) TestUpgradeFailsWithoutTools(c *gc.C) {
	machine, unit, _, _ := s.primeAgent(c)
	agent := s.newAgent(c, unit)
	newVers := version.Current
	newVers.Patch++
	err := machine.SetAgentVersion(newVers)
	c.Assert(err, gc.IsNil)
	err = runWithTimeout(agent)
	c.Assert(err, gc.ErrorMatches, "timed out waiting for agent to finish.*")
}

func (s *UnitSuite) TestWithDeadUnit(c *gc.C) {
	_, unit, _, _ := s.primeAgent(c)
	err := unit.EnsureDead()
	c.Assert(err, gc.IsNil)
	a := s.newAgent(c, unit)
	err = runWithTimeout(a)
	c.Assert(err, gc.IsNil)

	// try again when the unit has been removed.
	err = unit.Remove()
	c.Assert(err, gc.IsNil)
	a = s.newAgent(c, unit)
	err = runWithTimeout(a)
	c.Assert(err, gc.IsNil)
}

func (s *UnitSuite) TestOpenAPIState(c *gc.C) {
	_, unit, _, _ := s.primeAgent(c)
	s.testOpenAPIState(c, unit, s.newAgent(c, unit), initialUnitPassword)
}

func (s *UnitSuite) TestOpenAPIStateWithBadCredsTerminates(c *gc.C) {
	conf, _ := s.agentSuite.primeAgent(c, "unit-missing-0", "no-password", version.Current)
	_, _, err := openAPIState(conf, nil)
	c.Assert(err, gc.Equals, worker.ErrTerminateAgent)
}

type fakeUnitAgent struct {
	unitName string
}

func (f *fakeUnitAgent) Tag() names.Tag {
	return names.NewUnitTag(f.unitName)
}

func (f *fakeUnitAgent) ChangeConfig(func(agent.ConfigSetter)) error {
	panic("fakeUnitAgent.ChangeConfig called unexpectedly")
}

func (s *UnitSuite) TestOpenAPIStateWithDeadEntityTerminates(c *gc.C) {
	_, unit, conf, _ := s.primeAgent(c)
	err := unit.EnsureDead()
	c.Assert(err, gc.IsNil)
	_, _, err = openAPIState(conf, &fakeUnitAgent{"wordpress/0"})
	c.Assert(err, gc.Equals, worker.ErrTerminateAgent)
}

func (s *UnitSuite) TestOpenStateFails(c *gc.C) {
	// Start a unit agent and make sure it doesn't set a mongo password
	// we can use to connect to state with.
	_, unit, conf, _ := s.primeAgent(c)
	a := s.newAgent(c, unit)
	go func() { c.Check(a.Run(nil), gc.IsNil) }()
	defer func() { c.Check(a.Stop(), gc.IsNil) }()
	waitForUnitStarted(s.State, unit, c)

	s.assertCannotOpenState(c, conf.Tag(), conf.DataDir())
}

func (s *UnitSuite) TestRsyslogConfigWorker(c *gc.C) {
	created := make(chan rsyslog.RsyslogMode, 1)
	s.PatchValue(&newRsyslogConfigWorker, func(_ *apirsyslog.State, _ agent.Config, mode rsyslog.RsyslogMode) (worker.Worker, error) {
		created <- mode
		return newDummyWorker(), nil
	})

	_, unit, _, _ := s.primeAgent(c)
	a := s.newAgent(c, unit)
	go func() { c.Check(a.Run(nil), gc.IsNil) }()
	defer func() { c.Check(a.Stop(), gc.IsNil) }()

	select {
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timeout while waiting for rsyslog worker to be created")
	case mode := <-created:
		c.Assert(mode, gc.Equals, rsyslog.RsyslogModeForwarding)
	}
}

func (s *UnitSuite) TestUnitAgentRunsAPIAddressUpdaterWorker(c *gc.C) {
	_, unit, _, _ := s.primeAgent(c)
	a := s.newAgent(c, unit)
	go func() { c.Check(a.Run(nil), gc.IsNil) }()
	defer func() { c.Check(a.Stop(), gc.IsNil) }()

	// Update the API addresses.
	updatedServers := [][]network.HostPort{network.AddressesWithPort(
		network.NewAddresses("localhost"), 1234,
	)}
	err := s.BackingState.SetAPIHostPorts(updatedServers)
	c.Assert(err, gc.IsNil)

	// Wait for config to be updated.
	s.BackingState.StartSync()
	for attempt := coretesting.LongAttempt.Start(); attempt.Next(); {
		addrs, err := a.CurrentConfig().APIAddresses()
		c.Assert(err, gc.IsNil)
		if reflect.DeepEqual(addrs, []string{"localhost:1234"}) {
			return
		}
	}
	c.Fatalf("timeout while waiting for agent config to change")
}
