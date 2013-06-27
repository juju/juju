// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs/agent"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/testing"
	"time"
)

type UnitSuite struct {
	testing.GitSuite
	agentSuite
}

var _ = Suite(&UnitSuite{})

func (s *UnitSuite) SetUpTest(c *C) {
	s.GitSuite.SetUpTest(c)
	s.agentSuite.SetUpTest(c)
}

func (s *UnitSuite) TearDownTest(c *C) {
	s.agentSuite.TearDownTest(c)
	s.GitSuite.TearDownTest(c)
}

// primeAgent creates a unit, and sets up the unit agent's directory.
// It returns the new unit and the agent's configuration.
func (s *UnitSuite) primeAgent(c *C) (*state.Unit, *agent.Conf, *state.Tools) {
	svc, err := s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, IsNil)
	unit, err := svc.AddUnit()
	c.Assert(err, IsNil)
	err = unit.SetMongoPassword("unit-password")
	c.Assert(err, IsNil)
	conf, tools := s.agentSuite.primeAgent(c, unit.Tag(), "unit-password")
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
	unit, _, _ := s.primeAgent(c)
	a := s.newAgent(c, unit)
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
			case params.StatusPending, params.StatusInstalled:
				c.Logf("waiting...")
				continue
			case params.StatusStarted:
				c.Logf("started!")
				break waitStarted
			case params.StatusDown:
				s.State.StartSync()
				c.Logf("unit is still down")
			default:
				c.Fatalf("unexpected status %s %s", st, info)
			}
		}
	}
}

func (s *UnitSuite) TestUpgrade(c *C) {
	unit, _, currentTools := s.primeAgent(c)
	a := s.newAgent(c, unit)
	s.testUpgrade(c, a, currentTools)
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

func (s *UnitSuite) TestOpenAPIState(c *C) {
	c.Skip("unit agent API connection not implemented yet")
	unit, _, _ := s.primeAgent(c)
	s.testOpenAPIState(c, unit, s.newAgent(c, unit))
}
