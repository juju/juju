package main_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/agent"
	main "launchpad.net/juju/go/cmd/jujud"
)

type UnitSuite struct{}

var _ = Suite(&UnitSuite{})

func (s *UnitSuite) TestParseSuccess(c *C) {
	f := main.NewUnitFlags()
	err := ParseAgentFlags(c, f, []string{"--unit-name", "wordpress/1"})
	c.Assert(err, IsNil)
	agent, ok := f.Agent().(*agent.Unit)
	c.Assert(ok, Equals, true)
	c.Assert(agent.Name, Equals, "wordpress/1")
}

func (s *UnitSuite) TestParseMissing(c *C) {
	f := main.NewUnitFlags()
	err := ParseAgentFlags(c, f, []string{})
	c.Assert(err, ErrorMatches, "--unit-name option must be set")
}

func (s *UnitSuite) TestParseUnknown(c *C) {
	f := main.NewUnitFlags()
	err := ParseAgentFlags(c, f, []string{"--unit-name", "wordpress/1", "thundering typhoons"})
	c.Assert(err, ErrorMatches, `unrecognised args: \[thundering typhoons\]`)
}
