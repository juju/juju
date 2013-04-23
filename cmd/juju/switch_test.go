package main

import (
	. "launchpad.net/gocheck"
	_ "launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/testing"
)

type SwitchSimpleSuite struct {
}

var _ = Suite(&SwitchSimpleSuite{})

func (*SwitchSimpleSuite) TestNoEnvironment(c *C) {
	defer testing.MakeEmptyFakeHome(c).Restore()
	_, err := testing.RunCommand(c, &SwitchCommand{}, nil)
	c.Assert(err, ErrorMatches, "Couldn't read the environment.")
}

func (*SwitchSimpleSuite) TestNoDefault(c *C) {
	defer testing.MakeFakeHome(c, testing.MultipleEnvConfigNoDefault).Restore()
	context, err := testing.RunCommand(c, &SwitchCommand{}, nil)
	c.Assert(err, IsNil)
	c.Assert(testing.Stdout(context), Equals, "Current environment: <not specified>\n")
}
