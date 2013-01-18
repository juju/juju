package main

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs/agent"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/version"
	"time"
)

type UnitSuite struct {
	agentSuite
}

var _ = Suite(&UnitSuite{})

// primeAgent creates a unit, and sets up the unit agent's directory.
// It returns the new unit and the agent's configuration.
func (s *UnitSuite) primeAgent(c *C) (*state.Unit, *agent.Conf, *state.Tools) {
	svc, err := s.Conn.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, IsNil)
	unit, err := svc.AddUnit()
	c.Assert(err, IsNil)
	err = unit.SetMongoPassword("unit-password")
	c.Assert(err, IsNil)
	conf, tools := s.agentSuite.primeAgent(c, unit.EntityName(), "unit-password")
	return unit, conf, tools
}

func (s *UnitSuite) newAgent(c *C, unit *state.Unit) *UnitAgent {
	a := &UnitAgent{}
	s.initAgent(c, a, "--unit-name", unit.Name())
	return a
}

func (s *UnitSuite) TestParseSuccess(c *C) {
	create := func() (cmd.Command, *AgentConf) {
		a := &UnitAgent{}
		return a, &a.Conf
	}
	uc := CheckAgentCommand(c, create, []string{"--unit-name", "w0rd-pre55/1"})
	c.Assert(uc.(*UnitAgent).UnitName, Equals, "w0rd-pre55/1")
}

func (s *UnitSuite) TestParseMissing(c *C) {
	uc := &UnitAgent{}
	err := ParseAgentCommand(uc, []string{})
	c.Assert(err, ErrorMatches, "--unit-name option must be set")
}

func (s *UnitSuite) TestParseNonsense(c *C) {
	for _, args := range [][]string{
		{"--unit-name", "wordpress"},
		{"--unit-name", "wordpress/seventeen"},
		{"--unit-name", "wordpress/-32"},
		{"--unit-name", "wordpress/wild/9"},
		{"--unit-name", "20/20"},
	} {
		err := ParseAgentCommand(&UnitAgent{}, args)
		c.Assert(err, ErrorMatches, `--unit-name option expects "<service>/<n>" argument`)
	}
}

func (s *UnitSuite) TestParseUnknown(c *C) {
	uc := &UnitAgent{}
	err := ParseAgentCommand(uc, []string{"--unit-name", "wordpress/1", "thundering typhoons"})
	c.Assert(err, ErrorMatches, `unrecognized args: \["thundering typhoons"\]`)
}

func (s *UnitSuite) TestRunStop(c *C) {
	unit, conf, _ := s.primeAgent(c)
	a := s.newAgent(c, unit)
	mgr, reset := patchDeployManager(c, &conf.StateInfo, conf.DataDir)
	defer reset()
	go func() { c.Check(a.Run(nil), IsNil) }()
	defer func() { c.Check(a.Stop(), IsNil) }()
	timeout := time.After(5 * time.Second)

waitStarted:
	for {
		select {
		case <-timeout:
			c.Fatalf("no activity detected")
		case <-time.After(50 * time.Millisecond):
			err := unit.Refresh()
			c.Assert(err, IsNil)
			st, info, err := unit.Status()
			c.Assert(err, IsNil)
			switch st {
			case state.UnitPending, state.UnitInstalled:
				c.Logf("waiting...")
				continue
			case state.UnitStarted:
				c.Logf("started!")
				break waitStarted
			case state.UnitDown:
				s.State.StartSync()
				c.Logf("unit is still down")
			default:
				c.Fatalf("unexpected status %s %s", st, info)
			}
		}
	}

	// Check no subordinates have been deployed.
	mgr.waitDeployed(c)

	// Add a relation with a subordinate service and wait for the subordinate
	// to be deployed...
	_, err := s.State.AddService("logging", s.AddTestingCharm(c, "logging"))
	c.Assert(err, IsNil)
	eps, err := s.State.InferEndpoints([]string{"wordpress", "logging"})
	c.Assert(err, IsNil)
	_, err = s.State.AddRelation(eps...)
	c.Assert(err, IsNil)
	mgr.waitDeployed(c, "logging/0")

	// ...then kill the subordinate and wait for it to be recalled and removed.
	logging0, err := s.State.Unit("logging/0")
	c.Assert(err, IsNil)
	err = logging0.EnsureDead()
	c.Assert(err, IsNil)
	mgr.waitDeployed(c)
	err = logging0.Refresh()
	c.Assert(state.IsNotFound(err), Equals, true)
}

func (s *UnitSuite) TestUpgrade(c *C) {
	newVers := version.Current
	newVers.Patch++
	newTools := s.uploadTools(c, newVers)
	s.proposeVersion(c, newVers.Number, true)
	unit, _, currentTools := s.primeAgent(c)
	a := s.newAgent(c, unit)
	defer a.Stop()
	err := runWithTimeout(a)
	c.Assert(err, FitsTypeOf, &UpgradeReadyError{})
	ug := err.(*UpgradeReadyError)
	c.Assert(ug.NewTools, DeepEquals, newTools)
	c.Assert(ug.OldTools, DeepEquals, currentTools)
}

func (s *UnitSuite) TestWithDeadUnit(c *C) {
	unit, _, _ := s.primeAgent(c)
	err := unit.EnsureDead()
	c.Assert(err, IsNil)
	a := s.newAgent(c, unit)
	err = runWithTimeout(a)
	c.Assert(err, IsNil)

	// try again when the unit has been removed.
	err = unit.Remove()
	c.Assert(err, IsNil)
	a = s.newAgent(c, unit)
	err = runWithTimeout(a)
	c.Assert(err, IsNil)
}

func (s *UnitSuite) TestChangePasswordChanging(c *C) {
	unit, _, _ := s.primeAgent(c)
	newAgent := func() runner {
		return s.newAgent(c, unit)
	}
	s.testAgentPasswordChanging(c, unit, newAgent)
}
