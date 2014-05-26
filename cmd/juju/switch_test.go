// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"os"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cmd/envcmd"
	_ "launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/testing"
)

type SwitchSimpleSuite struct {
	testing.FakeJujuHomeSuite
}

var _ = gc.Suite(&SwitchSimpleSuite{})

func (*SwitchSimpleSuite) TestNoEnvironment(c *gc.C) {
	envPath := testing.HomePath(".juju", "environments.yaml")
	err := os.Remove(envPath)
	c.Assert(err, gc.IsNil)
	_, err = testing.RunCommand(c, &SwitchCommand{})
	c.Assert(err, gc.ErrorMatches, "couldn't read the environment")
}

func (*SwitchSimpleSuite) TestNoDefault(c *gc.C) {
	testing.WriteEnvironments(c, testing.MultipleEnvConfigNoDefault)
	_, err := testing.RunCommand(c, &SwitchCommand{})
	c.Assert(err, gc.ErrorMatches, "no currently specified environment")
}

func (*SwitchSimpleSuite) TestShowsDefault(c *gc.C) {
	testing.WriteEnvironments(c, testing.MultipleEnvConfig)
	context, err := testing.RunCommand(c, &SwitchCommand{})
	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stdout(context), gc.Equals, "erewhemos\n")
}

func (s *SwitchSimpleSuite) TestCurrentEnvironmentHasPrecidence(c *gc.C) {
	testing.WriteEnvironments(c, testing.MultipleEnvConfig)
	s.FakeHomeSuite.Home.AddFiles(c, testing.TestFile{".juju/current-environment", "fubar"})
	context, err := testing.RunCommand(c, &SwitchCommand{})
	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stdout(context), gc.Equals, "fubar\n")
}

func (*SwitchSimpleSuite) TestShowsJujuEnv(c *gc.C) {
	testing.WriteEnvironments(c, testing.MultipleEnvConfig)
	os.Setenv("JUJU_ENV", "using-env")
	context, err := testing.RunCommand(c, &SwitchCommand{})
	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stdout(context), gc.Equals, "using-env\n")
}

func (s *SwitchSimpleSuite) TestJujuEnvOverCurrentEnvironment(c *gc.C) {
	testing.WriteEnvironments(c, testing.MultipleEnvConfig)
	s.FakeHomeSuite.Home.AddFiles(c, testing.TestFile{".juju/current-environment", "fubar"})
	os.Setenv("JUJU_ENV", "using-env")
	context, err := testing.RunCommand(c, &SwitchCommand{})
	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stdout(context), gc.Equals, "using-env\n")
}

func (*SwitchSimpleSuite) TestSettingWritesFile(c *gc.C) {
	testing.WriteEnvironments(c, testing.MultipleEnvConfig)
	context, err := testing.RunCommand(c, &SwitchCommand{}, "erewhemos-2")
	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stdout(context), gc.Equals, "erewhemos -> erewhemos-2\n")
	c.Assert(envcmd.ReadCurrentEnvironment(), gc.Equals, "erewhemos-2")
}

func (*SwitchSimpleSuite) TestSettingToUnknown(c *gc.C) {
	testing.WriteEnvironments(c, testing.MultipleEnvConfig)
	_, err := testing.RunCommand(c, &SwitchCommand{}, "unknown")
	c.Assert(err, gc.ErrorMatches, `"unknown" is not a name of an existing defined environment`)
}

func (*SwitchSimpleSuite) TestSettingWhenJujuEnvSet(c *gc.C) {
	testing.WriteEnvironments(c, testing.MultipleEnvConfig)
	os.Setenv("JUJU_ENV", "using-env")
	_, err := testing.RunCommand(c, &SwitchCommand{}, "erewhemos-2")
	c.Assert(err, gc.ErrorMatches, `cannot switch when JUJU_ENV is overriding the environment \(set to "using-env"\)`)
}

const expectedEnvironments = `erewhemos
erewhemos-2
`

func (*SwitchSimpleSuite) TestListEnvironments(c *gc.C) {
	testing.WriteEnvironments(c, testing.MultipleEnvConfig)
	context, err := testing.RunCommand(c, &SwitchCommand{}, "--list")
	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stdout(context), gc.Equals, expectedEnvironments)
}

func (*SwitchSimpleSuite) TestListEnvironmentsOSJujuEnvSet(c *gc.C) {
	testing.WriteEnvironments(c, testing.MultipleEnvConfig)
	os.Setenv("JUJU_ENV", "using-env")
	context, err := testing.RunCommand(c, &SwitchCommand{}, "--list")
	c.Assert(err, gc.IsNil)
	c.Assert(testing.Stdout(context), gc.Equals, expectedEnvironments)
}

func (*SwitchSimpleSuite) TestListEnvironmentsAndChange(c *gc.C) {
	testing.WriteEnvironments(c, testing.MultipleEnvConfig)
	_, err := testing.RunCommand(c, &SwitchCommand{}, "--list", "erewhemos-2")
	c.Assert(err, gc.ErrorMatches, "cannot switch and list at the same time")
}

func (*SwitchSimpleSuite) TestTooManyParams(c *gc.C) {
	testing.WriteEnvironments(c, testing.MultipleEnvConfig)
	_, err := testing.RunCommand(c, &SwitchCommand{}, "foo", "bar")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: ."bar".`)
}
