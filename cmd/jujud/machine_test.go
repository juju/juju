package main_test

import (
	. "launchpad.net/gocheck"
	//"launchpad.net/juju/go/cmd"
	//main "launchpad.net/juju/go/cmd/jujud"
)

type MachineSuite struct{}

var _ = Suite(&MachineSuite{})

func (s *MachineSuite) TestFails(c *C) {
	c.Fail()
}
