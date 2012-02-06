package main_test

import (
	. "launchpad.net/gocheck"
	//"launchpad.net/juju/go/cmd"
	//main "launchpad.net/juju/go/cmd/jujud"
)

type ProvisioningSuite struct{}

var _ = Suite(&ProvisioningSuite{})

func (s *ProvisioningSuite) TestFails(c *C) {
	c.Fail()
}
