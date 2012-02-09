package main_test

import (
	. "launchpad.net/gocheck"
	main "launchpad.net/juju/go/cmd/jujud"
)

type MachineSuite struct{}

var _ = Suite(&MachineSuite{})

func (s *MachineSuite) TestParseSuccess(c *C) {
	create := func() main.AgentCommand { return main.NewMachineCommand() }
	mc := CheckAgentCommand(c, create, []string{"--machine-id", "42"})
	c.Assert(mc.(*main.MachineCommand).Agent.Id, Equals, uint(42))
}

func (s *MachineSuite) TestParseMissing(c *C) {
	mc := main.NewMachineCommand()
	err := ParseAgentCommand(mc, []string{})
	c.Assert(err, ErrorMatches, "--machine-id option must be set")
}

func (s *MachineSuite) TestParseNonsense(c *C) {
	for _, args := range [][]string{
		[]string{"--machine-id", "rastapopoulos"},
		[]string{"--machine-id", "-4004"},
		[]string{"--machine-id", "wordpress/2"},
	} {
		err := ParseAgentCommand(main.NewMachineCommand(), args)
		c.Assert(err, ErrorMatches, "--machine-id option expects a non-negative integer")
	}
}

func (s *MachineSuite) TestParseNegativeId(c *C) {
	mc := main.NewMachineCommand()
	err := ParseAgentCommand(mc, []string{"--machine-id", "-4004"})
	c.Assert(err, ErrorMatches, "--machine-id option expects a non-negative integer")
}

func (s *MachineSuite) TestParseUnknown(c *C) {
	mc := main.NewMachineCommand()
	err := ParseAgentCommand(mc, []string{"--machine-id", "42", "blistering barnacles"})
	c.Assert(err, ErrorMatches, `unrecognised args: \[blistering barnacles\]`)
}
