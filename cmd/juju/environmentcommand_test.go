// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"io/ioutil"
	"os"

	. "launchpad.net/gocheck"

	"launchpad.net/juju-core/juju/osenv"
	"launchpad.net/juju-core/testing"
)

type EnvironmentCommandSuite struct {
	home *testing.FakeHome
}

var _ = Suite(&EnvironmentCommandSuite{})

func (s *EnvironmentCommandSuite) SetUpTest(c *C) {
	s.home = testing.MakeEmptyFakeHome(c)
}

func (s *EnvironmentCommandSuite) TearDownTest(c *C) {
	s.home.Restore()
}

func (s *EnvironmentCommandSuite) TestReadCurrentEnvironmentUnset(c *C) {
	env := readCurrentEnvironment()
	c.Assert(env, Equals, "")
}

func (s *EnvironmentCommandSuite) TestReadCurrentEnvironmentSet(c *C) {
	err := writeCurrentEnvironment("fubar")
	c.Assert(err, IsNil)
	env := readCurrentEnvironment()
	c.Assert(env, Equals, "fubar")
}

func (s *EnvironmentCommandSuite) TestGetDefaultEnvironmentNothingSet(c *C) {
	env := getDefaultEnvironment()
	c.Assert(env, Equals, "")
}

func (s *EnvironmentCommandSuite) TestGetDefaultEnvironmentCurrentEnvironmentSet(c *C) {
	err := writeCurrentEnvironment("fubar")
	c.Assert(err, IsNil)
	env := getDefaultEnvironment()
	c.Assert(env, Equals, "fubar")
}

func (s *EnvironmentCommandSuite) TestGetDefaultEnvironmentJujuEnvSet(c *C) {
	os.Setenv(osenv.JujuEnv, "magic")
	env := getDefaultEnvironment()
	c.Assert(env, Equals, "magic")
}

func (s *EnvironmentCommandSuite) TestGetDefaultEnvironmentBothSet(c *C) {
	os.Setenv(osenv.JujuEnv, "magic")
	err := writeCurrentEnvironment("fubar")
	c.Assert(err, IsNil)
	env := getDefaultEnvironment()
	c.Assert(env, Equals, "magic")
}

func (s *EnvironmentCommandSuite) TestWriteAddsNewline(c *C) {
	err := writeCurrentEnvironment("fubar")
	c.Assert(err, IsNil)
	current, err := ioutil.ReadFile(getCurrentEnvironmentFilePath())
	c.Assert(err, IsNil)
	c.Assert(string(current), Equals, "fubar\n")
}

func (*EnvironmentCommandSuite) TestErrorWritingFile(c *C) {
	// Can't write a file over a directory.
	os.MkdirAll(getCurrentEnvironmentFilePath(), 0777)
	err := writeCurrentEnvironment("fubar")
	c.Assert(err, ErrorMatches, "unable to write to the environment file: .*")
}
