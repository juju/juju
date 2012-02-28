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
	c.Assert(a.(*main.MachineAgent).MachineId, Equals, 42)
}

func (s *MachineSuite) TestParseNonsense(c *C) {
	for _, args := range [][]string{
		[]string{},
		[]string{"--machine-id", "-4004"},
	} {
		err := ParseAgentCommand(main.NewMachineAgent(), args)
		c.Assert(err, ErrorMatches, "--machine-id option must be set, and expects a non-negative integer")
	}
}

func (s *MachineSuite) TestParseUnknown(c *C) {
	a := main.NewMachineAgent()
	err := ParseAgentCommand(a, []string{"--machine-id", "42", "blistering barnacles"})
	c.Assert(err, ErrorMatches, `unrecognised args: \[blistering barnacles\]`)
}
