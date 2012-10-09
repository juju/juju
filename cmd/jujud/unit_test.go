package main

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/version"
	"time"
)

type UnitSuite struct {
	testing.JujuConnSuite
}

var _ = Suite(&UnitSuite{})

func (s *UnitSuite) TestParseSuccess(c *C) {
	create := func() (cmd.Command, *AgentConf) {
		a := &UnitAgent{}
		return a, &a.Conf
	}
	uc := CheckAgentCommand(c, create, []string{"--unit-name", "w0rd-pre55/1"}, flagAll)
	c.Assert(uc.(*UnitAgent).UnitName, Equals, "w0rd-pre55/1")
}

func (s *UnitSuite) TestParseMissing(c *C) {
	uc := &UnitAgent{}
	err := ParseAgentCommand(uc, []string{})
	c.Assert(err, ErrorMatches, "--unit-name option must be set")
}

func (s *UnitSuite) TestParseNonsense(c *C) {
	for _, args := range [][]string{
		[]string{"--unit-name", "wordpress"},
		[]string{"--unit-name", "wordpress/seventeen"},
		[]string{"--unit-name", "wordpress/-32"},
		[]string{"--unit-name", "wordpress/wild/9"},
		[]string{"--unit-name", "20/20"},
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
	a, unit, _ := s.newAgent(c)
	done := make(chan error)
	go func() {
		done <- a.Run(nil)
	}()
	defer a.Stop()
	timeout := time.After(5 * time.Second)
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
			case state.UnitDown:
				s.State.StartSync()
				c.Logf("unit is still down")
			default:
				c.Fatalf("unexpected status %s %s", st, info)
			}
		}
		break
	}
	err := a.Stop()
	c.Assert(err, IsNil)
	c.Assert(<-done, IsNil)
}

// newAgent starts a new unit agent running a unit
// of the dummy charm.
func (s *UnitSuite) newAgent(c *C) (*UnitAgent, *state.Unit, *state.Tools) {
	ch := s.AddTestingCharm(c, "dummy")
	svc, err := s.Conn.AddService("dummy", ch)
	c.Assert(err, IsNil)
	unit, err := svc.AddUnit()
	c.Assert(err, IsNil)

	dataDir, tools := primeTools(c, s.Conn, version.Current)
	tools1, err := environs.ChangeAgentTools(dataDir, unit.EntityName(), version.Current)
	c.Assert(err, IsNil)
	c.Assert(tools1, DeepEquals, tools)

	return &UnitAgent{
		Conf: AgentConf{
			DataDir:   dataDir,
			StateInfo: *s.StateInfo(c),
		},
		UnitName: unit.Name(),
	}, unit, tools
}

func (s *UnitSuite) TestUpgrade(c *C) {
	newVers := version.Current
	newVers.Patch++
	newTools := uploadTools(c, s.Conn, newVers)
	proposeVersion(c, s.State, newVers.Number, true)
	a, _, currentTools := s.newAgent(c)
	defer a.Stop()
	err := runWithTimeout(a)
	c.Assert(err, FitsTypeOf, &UpgradeReadyError{})
	ug := err.(*UpgradeReadyError)
	c.Assert(ug.NewTools, DeepEquals, newTools)
	c.Assert(ug.OldTools, DeepEquals, currentTools)
}

func (s *UnitSuite) TestWithDeadUnit(c *C) {
	ch := s.AddTestingCharm(c, "dummy")
	svc, err := s.Conn.AddService("dummy", ch)
	c.Assert(err, IsNil)
	unit, err := svc.AddUnit()
	c.Assert(err, IsNil)
	err = unit.EnsureDead()
	c.Assert(err, IsNil)

	dataDir := c.MkDir()
	a := &UnitAgent{
		Conf: AgentConf{
			DataDir:   dataDir,
			StateInfo: *s.StateInfo(c),
		},
		UnitName: unit.Name(),
	}
	err = runWithTimeout(a)
	c.Assert(err, IsNil)

	// try again when the unit has been removed.
	err = svc.RemoveUnit(unit)
	c.Assert(err, IsNil)
	a = &UnitAgent{
		Conf: AgentConf{
			DataDir:   dataDir,
			StateInfo: *s.StateInfo(c),
		},
		UnitName: unit.Name(),
	}
	err = runWithTimeout(a)
	c.Assert(err, IsNil)
}

type runner interface {
	Run(*cmd.Context) error
	Stop() error
}

func runWithTimeout(runner runner) error {
	done := make(chan error)
	go func() {
		done <- runner.Run(nil)
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
	}
	err := runner.Stop()
	return fmt.Errorf("timed out waiting for agent to finish; stop error: %v", err)
}
