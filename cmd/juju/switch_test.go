package main

import (
	"os"

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

func (*SwitchSimpleSuite) TestShowsDefault(c *C) {
	defer testing.MakeFakeHome(c, testing.MultipleEnvConfig).Restore()
	context, err := testing.RunCommand(c, &SwitchCommand{}, nil)
	c.Assert(err, IsNil)
	c.Assert(testing.Stdout(context), Equals, "Current environment: \"erewhemos\"\n")
}

func (*SwitchSimpleSuite) TestCurrentEnvironmentHasPrecidence(c *C) {
	home := testing.MakeFakeHome(c, testing.MultipleEnvConfig)
	defer home.Restore()
	home.AddFiles(c, []testing.TestFile{{".juju/current-environment", "fubar"}})
	context, err := testing.RunCommand(c, &SwitchCommand{}, nil)
	c.Assert(err, IsNil)
	c.Assert(testing.Stdout(context), Equals, "Current environment: \"fubar\"\n")
}

func (*SwitchSimpleSuite) TestShowsJujuEnv(c *C) {
	defer testing.MakeFakeHome(c, testing.MultipleEnvConfig).Restore()
	os.Setenv("JUJU_ENV", "using-env")
	context, err := testing.RunCommand(c, &SwitchCommand{}, nil)
	c.Assert(err, IsNil)
	c.Assert(testing.Stdout(context), Equals, "Current environment: \"using-env\" (from JUJU_ENV)\n")
}

func (*SwitchSimpleSuite) TestJujuEnvOverCurrentEnvironment(c *C) {
	home := testing.MakeFakeHome(c, testing.MultipleEnvConfig)
	defer home.Restore()
	home.AddFiles(c, []testing.TestFile{{".juju/current-environment", "fubar"}})
	os.Setenv("JUJU_ENV", "using-env")
	context, err := testing.RunCommand(c, &SwitchCommand{}, nil)
	c.Assert(err, IsNil)
	c.Assert(testing.Stdout(context), Equals, "Current environment: \"using-env\" (from JUJU_ENV)\n")
}
