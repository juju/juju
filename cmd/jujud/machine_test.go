package main_test

import (
	. "launchpad.net/gocheck"
	main "launchpad.net/juju/go/cmd/jujud"
)

type MachineSuite struct{}

var _ = Suite(&MachineSuite{})

func (s *MachineSuite) TestParseSuccess(c *C) {
	create := func() main.AgentCommand { return main.NewMachineAgent() }
	a := CheckAgentCommand(c, create, []string{"--machine-id", "42"})
	c.Assert(a.(*main.MachineAgent).MachineId, Equals, uint(42))
}

func (s *MachineSuite) TestParseMissing(c *C) {
	a := main.NewMachineAgent()
	err := ParseAgentCommand(a, []string{})
	c.Assert(err, ErrorMatches, "--machine-id option must be set")
}

func (s *MachineSuite) TestParseNonsense(c *C) {
	for _, args := range [][]string{
		[]string{"--machine-id", "rastapopoulos"},
		[]string{"--machine-id", "-4004"},
		[]string{"--machine-id", "wordpress/2"},
	} {
		err := ParseAgentCommand(main.NewMachineAgent(), args)
		c.Assert(err, ErrorMatches, "--machine-id option expects a non-negative integer")
	}
}

func (s *MachineSuite) TestParseNegativeId(c *C) {
	a := main.NewMachineAgent()
	err := ParseAgentCommand(a, []string{"--machine-id", "-4004"})
	c.Assert(err, ErrorMatches, "--machine-id option expects a non-negative integer")
}

func (s *MachineSuite) TestParseUnknown(c *C) {
	a := main.NewMachineAgent()
	err := ParseAgentCommand(a, []string{"--machine-id", "42", "blistering barnacles"})
	c.Assert(err, ErrorMatches, `unrecognised args: \[blistering barnacles\]`)
}
