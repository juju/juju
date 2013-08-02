// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"os"

	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	_ "launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/testing"
)

type SwitchSimpleSuite struct {
}

var _ = Suite(&SwitchSimpleSuite{})

func (*SwitchSimpleSuite) TestNoEnvironment(c *C) {
	defer testing.MakeEmptyFakeHome(c).Restore()
	_, err := testing.RunCommand(c, &SwitchCommand{}, nil)
	c.Assert(err, ErrorMatches, "couldn't read the environment.")
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

func (*SwitchSimpleSuite) TestSettingWritesFile(c *C) {
	defer testing.MakeFakeHome(c, testing.MultipleEnvConfig).Restore()
	context, err := testing.RunCommand(c, &SwitchCommand{}, []string{"erewhemos-2"})
	c.Assert(err, IsNil)
	c.Assert(testing.Stdout(context), Equals, "Changed default environment from \"erewhemos\" to \"erewhemos-2\"\n")
	c.Assert(cmd.ReadCurrentEnvironment(), Equals, "erewhemos-2")
}

func (*SwitchSimpleSuite) TestSettingToUnknown(c *C) {
	defer testing.MakeFakeHome(c, testing.MultipleEnvConfig).Restore()
	_, err := testing.RunCommand(c, &SwitchCommand{}, []string{"unknown"})
	c.Assert(err, ErrorMatches, `"unknown" is not a name of an existing defined environment`)
}

func (*SwitchSimpleSuite) TestSettingWhenJujuEnvSet(c *C) {
	defer testing.MakeFakeHome(c, testing.MultipleEnvConfig).Restore()
	os.Setenv("JUJU_ENV", "using-env")
	_, err := testing.RunCommand(c, &SwitchCommand{}, []string{"erewhemos-2"})
	c.Assert(err, ErrorMatches, `Cannot switch when JUJU_ENV is overriding the environment \(set to "using-env"\)`)
}

const expectedEnvironments = `
Environments:
	erewhemos
	erewhemos-2
`

func (*SwitchSimpleSuite) TestListEnvironments(c *C) {
	defer testing.MakeFakeHome(c, testing.MultipleEnvConfig).Restore()
	context, err := testing.RunCommand(c, &SwitchCommand{}, []string{"--list"})
	c.Assert(err, IsNil)
	c.Assert(testing.Stdout(context), Matches, "Current environment: \"erewhemos\"(.|\n)*")
	c.Assert(testing.Stdout(context), Matches, "(.|\n)*"+expectedEnvironments)
}

func (*SwitchSimpleSuite) TestListEnvironmentsAndChange(c *C) {
	defer testing.MakeFakeHome(c, testing.MultipleEnvConfig).Restore()
	context, err := testing.RunCommand(c, &SwitchCommand{}, []string{"--list", "erewhemos-2"})
	c.Assert(err, IsNil)
	c.Assert(testing.Stdout(context), Matches, "Changed default environment from \"erewhemos\" to \"erewhemos-2\"(.|\n)*")
	c.Assert(testing.Stdout(context), Matches, "(.|\n)*"+expectedEnvironments)
}

func (*SwitchSimpleSuite) TestTooManyParams(c *C) {
	defer testing.MakeFakeHome(c, testing.MultipleEnvConfig).Restore()
	_, err := testing.RunCommand(c, &SwitchCommand{}, []string{"foo", "bar"})
	c.Assert(err, ErrorMatches, `unrecognized args: ."bar".`)
}
