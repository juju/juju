package main_test

import (
	. "launchpad.net/gocheck"
	//"launchpad.net/juju/go/cmd"
	//main "launchpad.net/juju/go/cmd/jujud"
)

type UnitSuite struct{}

var _ = Suite(&UnitSuite{})

func (s *UnitSuite) TestFails(c *C) {
	c.Fail()
}
