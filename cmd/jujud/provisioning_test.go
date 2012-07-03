package main

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/testing"
)

type ProvisioningSuite struct {
	testing.LoggingSuite
}

var _ = Suite(&ProvisioningSuite{})

func (s *ProvisioningSuite) TestParseSuccess(c *C) {
	create := func() (cmd.Command, *AgentConf) {
		a := &ProvisioningAgent{}
		return a, &a.Conf
	}
	CheckAgentCommand(c, create, []string{})
}

func (s *ProvisioningSuite) TestParseUnknown(c *C) {
	a := &ProvisioningAgent{}
	err := ParseAgentCommand(a, []string{"nincompoops"})
	c.Assert(err, ErrorMatches, `unrecognized args: \["nincompoops"\]`)
}
