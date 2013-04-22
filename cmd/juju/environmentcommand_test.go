package main

import (
	"io/ioutil"
	"os"
	"path/filepath"

	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/juju"
)

type EnvironmentCommandSuite struct {
	originalJujuHome string
	originalJujuEnv  string
	jujuHome         string
}

var _ = Suite(&EnvironmentCommandSuite{})

func (s *EnvironmentCommandSuite) SetUpTest(c *C) {
	s.originalJujuHome = os.Getenv("JUJU_HOME")
	s.originalJujuEnv = os.Getenv("JUJU_ENV")

	os.Setenv("JUJU_ENV", "")
	s.jujuHome = c.MkDir()
	os.Setenv("JUJU_HOME", s.jujuHome)
	err := juju.InitJujuHome()
	c.Assert(err, IsNil)
}

func (s *EnvironmentCommandSuite) TearDownTest(c *C) {
	os.Setenv("JUJU_HOME", s.originalJujuHome)
	os.Setenv("JUJU_ENV", s.originalJujuEnv)
}

func (s *EnvironmentCommandSuite) WriteCurrentEnvironment(c *C, env string) {
	path := filepath.Join(s.jujuHome, CurrentEnvironmentFile)
	err := ioutil.WriteFile(path, []byte(env), 0644)
	c.Assert(err, IsNil)
}

func (s *EnvironmentCommandSuite) TestReadCurrentEnvironmentUnset(c *C) {
	env := readCurrentEnvironment()
	c.Assert(env, Equals, "")
}

func (s *EnvironmentCommandSuite) TestReadCurrentEnvironmentSet(c *C) {
	s.WriteCurrentEnvironment(c, "fubar")
	env := readCurrentEnvironment()
	c.Assert(env, Equals, "fubar")
}

func (s *EnvironmentCommandSuite) TestGetDefaultEnvironmentNothingSet(c *C) {
	env := getDefaultEnvironment()
	c.Assert(env, Equals, "")
}

func (s *EnvironmentCommandSuite) TestGetDefaultEnvironmentCurrentEnvironmentSet(c *C) {
	s.WriteCurrentEnvironment(c, "fubar")
	env := getDefaultEnvironment()
	c.Assert(env, Equals, "fubar")
}

func (s *EnvironmentCommandSuite) TestGetDefaultEnvironmentJujuEnvSet(c *C) {
	os.Setenv("JUJU_ENV", "magic")
	env := getDefaultEnvironment()
	c.Assert(env, Equals, "magic")
}

func (s *EnvironmentCommandSuite) TestGetDefaultEnvironmentBothSet(c *C) {
	os.Setenv("JUJU_ENV", "magic")
	s.WriteCurrentEnvironment(c, "fubar")
	env := getDefaultEnvironment()
	c.Assert(env, Equals, "magic")
}
