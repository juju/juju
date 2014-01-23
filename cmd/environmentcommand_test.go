// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd_test

import (
	"io/ioutil"
	"os"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju/osenv"
	"launchpad.net/juju-core/testing"
)

type EnvironmentCommandSuite struct {
	home *testing.FakeHome
}

var _ = gc.Suite(&EnvironmentCommandSuite{})

func (s *EnvironmentCommandSuite) SetUpTest(c *gc.C) {
	s.home = testing.MakeEmptyFakeHome(c)
}

func (s *EnvironmentCommandSuite) TearDownTest(c *gc.C) {
	s.home.Restore()
}

func (s *EnvironmentCommandSuite) TestReadCurrentEnvironmentUnset(c *gc.C) {
	env := cmd.ReadCurrentEnvironment()
	c.Assert(env, gc.Equals, "")
}

func (s *EnvironmentCommandSuite) TestReadCurrentEnvironmentSet(c *gc.C) {
	err := cmd.WriteCurrentEnvironment("fubar")
	c.Assert(err, gc.IsNil)
	env := cmd.ReadCurrentEnvironment()
	c.Assert(env, gc.Equals, "fubar")
}

func (s *EnvironmentCommandSuite) TestGetDefaultEnvironmentNothingSet(c *gc.C) {
	env := cmd.GetDefaultEnvironment()
	c.Assert(env, gc.Equals, "")
}

func (s *EnvironmentCommandSuite) TestGetDefaultEnvironmentCurrentEnvironmentSet(c *gc.C) {
	err := cmd.WriteCurrentEnvironment("fubar")
	c.Assert(err, gc.IsNil)
	env := cmd.GetDefaultEnvironment()
	c.Assert(env, gc.Equals, "fubar")
}

func (s *EnvironmentCommandSuite) TestGetDefaultEnvironmentJujuEnvSet(c *gc.C) {
	os.Setenv(osenv.JujuEnvEnvKey, "magic")
	env := cmd.GetDefaultEnvironment()
	c.Assert(env, gc.Equals, "magic")
}

func (s *EnvironmentCommandSuite) TestGetDefaultEnvironmentBothSet(c *gc.C) {
	os.Setenv(osenv.JujuEnvEnvKey, "magic")
	err := cmd.WriteCurrentEnvironment("fubar")
	c.Assert(err, gc.IsNil)
	env := cmd.GetDefaultEnvironment()
	c.Assert(env, gc.Equals, "magic")
}

func (s *EnvironmentCommandSuite) TestWriteAddsNewline(c *gc.C) {
	err := cmd.WriteCurrentEnvironment("fubar")
	c.Assert(err, gc.IsNil)
	current, err := ioutil.ReadFile(cmd.GetCurrentEnvironmentFilePath())
	c.Assert(err, gc.IsNil)
	c.Assert(string(current), gc.Equals, "fubar\n")
}

func (*EnvironmentCommandSuite) TestErrorWritingFile(c *gc.C) {
	// Can't write a file over a directory.
	os.MkdirAll(cmd.GetCurrentEnvironmentFilePath(), 0777)
	err := cmd.WriteCurrentEnvironment("fubar")
	c.Assert(err, gc.ErrorMatches, "unable to write to the environment file: .*")
}
