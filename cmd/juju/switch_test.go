// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"os"

	gc "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	_ "launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/testing"
)

type SwitchSimpleSuite struct {
}

var _ = gc.Suite(&SwitchSimpleSuite{})

func (*SwitchSimpleSuite) TestNoEnvironment(c *gc.C) {
	defer testing.MakeEmptyFakeHome(c).Restore()
	_, err := testing.RunCommand(c, &SwitchCommand{}, nil)
	c.Assert(err, gc.ErrorMatches, "couldn't read the environment.")
}

func (*SwitchSimpleSuite) TestNoDefault(c *gc.C) {
	defer testing.MakeFakeHome(c, testing.MultipleEnvConfigNoDefault).Restore()
	context, err := testing.RunCommand(c, &SwitchCommand{}, nil)
	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stdout(context), gc.Equals, "Current environment: <not specified>\n")
}

func (*SwitchSimpleSuite) TestShowsDefault(c *gc.C) {
	defer testing.MakeFakeHome(c, testing.MultipleEnvConfig).Restore()
	context, err := testing.RunCommand(c, &SwitchCommand{}, nil)
	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stdout(context), gc.Equals, "Current environment: \"erewhemos\"\n")
}

func (*SwitchSimpleSuite) TestCurrentEnvironmentHasPrecidence(c *gc.C) {
	home := testing.MakeFakeHome(c, testing.MultipleEnvConfig)
	defer home.Restore()
	home.AddFiles(c, []testing.TestFile{{".juju/current-environment", "fubar"}})
	context, err := testing.RunCommand(c, &SwitchCommand{}, nil)
	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stdout(context), gc.Equals, "Current environment: \"fubar\"\n")
}

func (*SwitchSimpleSuite) TestShowsJujuEnv(c *gc.C) {
	defer testing.MakeFakeHome(c, testing.MultipleEnvConfig).Restore()
	os.Setenv("JUJU_ENV", "using-env")
	context, err := testing.RunCommand(c, &SwitchCommand{}, nil)
	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stdout(context), gc.Equals, "Current environment: \"using-env\" (from JUJU_ENV)\n")
}

func (*SwitchSimpleSuite) TestJujuEnvOverCurrentEnvironment(c *gc.C) {
	home := testing.MakeFakeHome(c, testing.MultipleEnvConfig)
	defer home.Restore()
	home.AddFiles(c, []testing.TestFile{{".juju/current-environment", "fubar"}})
	os.Setenv("JUJU_ENV", "using-env")
	context, err := testing.RunCommand(c, &SwitchCommand{}, nil)
	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stdout(context), gc.Equals, "Current environment: \"using-env\" (from JUJU_ENV)\n")
}

func (*SwitchSimpleSuite) TestSettingWritesFile(c *gc.C) {
	defer testing.MakeFakeHome(c, testing.MultipleEnvConfig).Restore()
	context, err := testing.RunCommand(c, &SwitchCommand{}, []string{"erewhemos-2"})
	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stdout(context), gc.Equals, "Changed default environment from \"erewhemos\" to \"erewhemos-2\"\n")
	c.Assert(cmd.ReadCurrentEnvironment(), gc.Equals, "erewhemos-2")
}

func (*SwitchSimpleSuite) TestSettingToUnknown(c *gc.C) {
	defer testing.MakeFakeHome(c, testing.MultipleEnvConfig).Restore()
	_, err := testing.RunCommand(c, &SwitchCommand{}, []string{"unknown"})
	c.Assert(err, gc.ErrorMatches, `"unknown" is not a name of an existing defined environment`)
}

func (*SwitchSimpleSuite) TestSettingWhenJujuEnvSet(c *gc.C) {
	defer testing.MakeFakeHome(c, testing.MultipleEnvConfig).Restore()
	os.Setenv("JUJU_ENV", "using-env")
	_, err := testing.RunCommand(c, &SwitchCommand{}, []string{"erewhemos-2"})
	c.Assert(err, gc.ErrorMatches, `Cannot switch when JUJU_ENV is overriding the environment \(set to "using-env"\)`)
}

const expectedEnvironments = `
Environments:
	erewhemos
	erewhemos-2
`

func (*SwitchSimpleSuite) TestListEnvironments(c *gc.C) {
	defer testing.MakeFakeHome(c, testing.MultipleEnvConfig).Restore()
	context, err := testing.RunCommand(c, &SwitchCommand{}, []string{"--list"})
	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stdout(context), gc.Matches, "Current environment: \"erewhemos\"(.|\n)*")
	c.Assert(testing.Stdout(context), gc.Matches, "(.|\n)*"+expectedEnvironments)
}

func (*SwitchSimpleSuite) TestListEnvironmentsAndChange(c *gc.C) {
	defer testing.MakeFakeHome(c, testing.MultipleEnvConfig).Restore()
	context, err := testing.RunCommand(c, &SwitchCommand{}, []string{"--list", "erewhemos-2"})
	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stdout(context), gc.Matches, "Changed default environment from \"erewhemos\" to \"erewhemos-2\"(.|\n)*")
	c.Assert(testing.Stdout(context), gc.Matches, "(.|\n)*"+expectedEnvironments)
}

func (*SwitchSimpleSuite) TestTooManyParams(c *gc.C) {
	defer testing.MakeFakeHome(c, testing.MultipleEnvConfig).Restore()
	_, err := testing.RunCommand(c, &SwitchCommand{}, []string{"foo", "bar"})
	c.Assert(err, gc.ErrorMatches, `unrecognized args: ."bar".`)
}
