package main_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/agent"
	main "launchpad.net/juju/go/cmd/jujud"
)

type ProvisioningSuite struct{}

var _ = Suite(&ProvisioningSuite{})

func (s *ProvisioningSuite) TestParseSuccess(c *C) {
	f := main.NewProvisioningFlags()
	err := ParseAgentFlags(c, f, []string{})
	c.Assert(err, IsNil)
	_, ok := f.Agent().(*agent.Provisioning)
	c.Assert(ok, Equals, true)
}

func (s *ProvisioningSuite) TestParseUnknown(c *C) {
	f := main.NewProvisioningFlags()
	err := ParseAgentFlags(c, f, []string{"nincompoops"})
	c.Assert(err, ErrorMatches, `unrecognised args: \[nincompoops\]`)
}
