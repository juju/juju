package main

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
)

type MachineSuite struct {
	testing.JujuConnSuite
}

var _ = Suite(&MachineSuite{})

func (s *MachineSuite) TestParseSuccess(c *C) {
	create := func() (cmd.Command, *AgentConf) {
		a := &MachineAgent{}
		return a, &a.Conf
	}
	a := CheckAgentCommand(c, create, []string{"--machine-id", "42"})
	c.Assert(a.(*MachineAgent).MachineId, Equals, 42)
}

func (s *MachineSuite) TestParseNonsense(c *C) {
	for _, args := range [][]string{
		[]string{},
		[]string{"--machine-id", "-4004"},
	} {
		err := ParseAgentCommand(&MachineAgent{}, args)
		c.Assert(err, ErrorMatches, "--machine-id option must be set, and expects a non-negative integer")
	}
}

func (s *MachineSuite) TestParseUnknown(c *C) {
	a := &MachineAgent{}
	err := ParseAgentCommand(a, []string{"--machine-id", "42", "blistering barnacles"})
	c.Assert(err, ErrorMatches, `unrecognized args: \["blistering barnacles"\]`)
}

func (s *MachineSuite) TestRunInvalidMachineId(c *C) {
	c.Skip("agents don't yet distinguish between temporary and permanent errors")
	a := &MachineAgent{
		Conf: AgentConf{
			DataDir:   c.MkDir(),
			StateInfo: *s.StateInfo(c),
		},
		MachineId: 2,
	}
	err := a.Run(nil)
	c.Assert(err, ErrorMatches, "some error")
}

func (s *MachineSuite) TestRunStop(c *C) {
	m, err := s.State.AddMachine(state.MachineWorker)
	c.Assert(err, IsNil)
	a := &MachineAgent{
		Conf: AgentConf{
			DataDir:   c.MkDir(),
			StateInfo: *s.StateInfo(c),
		},
		MachineId: m.Id(),
	}
	done := make(chan error)
	go func() {
		done <- a.Run(nil)
	}()
	err = a.Stop()
	c.Assert(err, IsNil)
	c.Assert(<-done, IsNil)
}
