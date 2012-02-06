package main_test

import (
	. "launchpad.net/gocheck"
	//"launchpad.net/juju/go/cmd"
	//main "launchpad.net/juju/go/cmd/jujud"
)

type AgentSuite struct{}

var _ = Suite(&AgentSuite{})

func (s *AgentSuite) TestFails(c *C) {
	c.Fail()
}
