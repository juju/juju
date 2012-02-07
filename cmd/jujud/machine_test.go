package main_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/agent"
	main "launchpad.net/juju/go/cmd/jujud"
)

type MachineSuite struct{}

var _ = Suite(&MachineSuite{})

func (s *MachineSuite) TestParseSuccess(c *C) {
	f := main.NewMachineFlags()
	err := ParseAgentFlags(c, f, []string{"--machine-id", "42"})
	c.Assert(err, IsNil)
	agent, ok := f.Agent().(*agent.Machine)
	c.Assert(ok, Equals, true)
	c.Assert(agent.Id, Equals, uint(42))
}

func (s *MachineSuite) TestParseMissing(c *C) {
	f := main.NewMachineFlags()
	err := ParseAgentFlags(c, f, []string{})
	c.Assert(err, ErrorMatches, "--machine-id option must be set")
}

func (s *MachineSuite) TestParseNonsenseId(c *C) {
	f := main.NewMachineFlags()
	err := ParseAgentFlags(c, f, []string{"--machine-id", "rastapopoulos"})
	c.Assert(err, ErrorMatches, "--machine-id option expects a non-negative integer")
}

func (s *MachineSuite) TestParseNegativeId(c *C) {
	f := main.NewMachineFlags()
	err := ParseAgentFlags(c, f, []string{"--machine-id", "-4004"})
	c.Assert(err, ErrorMatches, "--machine-id option expects a non-negative integer")
}

func (s *MachineSuite) TestParseUnknown(c *C) {
	f := main.NewMachineFlags()
	err := ParseAgentFlags(c, f, []string{"blistering barnacles"})
	c.Assert(err, ErrorMatches, `unrecognised args: \[blistering barnacles\]`)
}
