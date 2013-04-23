package main

import (
	. "launchpad.net/gocheck"
	_ "launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/testing"
)

type SwitchSimpleSuite struct {
}

var _ = Suite(&SwitchSimpleSuite{})

func (*SwitchSimpleSuite) TestNoDefault(c *C) {
	defer testing.MakeFakeHome(c, testing.SingleEnvConfigNoDefault, testing.SampleCertName).Restore()
	//err := juju.InitJujuHome()
	//c.Assert(err, IsNil)
	//context, err := testing.RunCommand(c, &SwitchCommand{}, nil)
	//c.Assert(err, IsNil)
	//c.Assert(testing.Stdout(context), Equals, "Current environment: <not specified>\n")
}
