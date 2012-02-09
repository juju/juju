package main_test

import (
	. "launchpad.net/gocheck"
	main "launchpad.net/juju/go/cmd/jujud"
)

type ProvisioningSuite struct{}

var _ = Suite(&ProvisioningSuite{})

func (s *ProvisioningSuite) TestParseSuccess(c *C) {
	create := func() main.AgentCommand { return main.NewProvisioningCommand() }
	CheckAgentCommand(c, create, []string{})
}

func (s *ProvisioningSuite) TestParseUnknown(c *C) {
	pc := main.NewProvisioningCommand()
	err := ParseAgentCommand(pc, []string{"nincompoops"})
	c.Assert(err, ErrorMatches, `unrecognised args: \[nincompoops\]`)
}
