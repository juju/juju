package main_test

import (
	. "launchpad.net/gocheck"
	//"launchpad.net/juju/go/cmd"
	//main "launchpad.net/juju/go/cmd/jujud"
)

type InitzkSuite struct{}

var _ = Suite(&InitzkSuite{})

func (s *InitzkSuite) TestFails(c *C) {
	c.Fail()
}
