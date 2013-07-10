// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing_test

import (
	"os"
	"path/filepath"

	. "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/testing"
)

type TestingEnvironSuite struct {
	home     string
	jujuHome string
}

var _ = Suite(&TestingEnvironSuite{})

func (s *TestingEnvironSuite) SetUpTest(c *C) {
	s.home = os.Getenv("HOME")
	s.jujuHome = os.Getenv("JUJU_HOME")

	os.Setenv("HOME", "/home/eric")
	os.Setenv("JUJU_HOME", "/home/eric/juju")
	config.SetJujuHome("/home/eric/juju")
}

func (s *TestingEnvironSuite) TearDownTest(c *C) {
	os.Setenv("HOME", s.home)
	os.Setenv("JUJU_HOME", s.jujuHome)
}

func (s *TestingEnvironSuite) TestFakeHomeReplacesEnvironment(c *C) {
	_ = testing.MakeEmptyFakeHome(c)
	c.Assert(os.Getenv("HOME"), Not(Equals), "/home/eric")
	c.Assert(os.Getenv("JUJU_HOME"), Equals, "")
	c.Assert(config.JujuHome(), Not(Equals), "/home/eric/juju")
}

func (s *TestingEnvironSuite) TestFakeHomeRestoresEnvironment(c *C) {
	fake := testing.MakeEmptyFakeHome(c)
	fake.Restore()
	c.Assert(os.Getenv("HOME"), Equals, "/home/eric")
	c.Assert(os.Getenv("JUJU_HOME"), Equals, "/home/eric/juju")
	c.Assert(config.JujuHome(), Equals, "/home/eric/juju")
}

func (s *TestingEnvironSuite) TestFakeHomeSetsConfigJujuHome(c *C) {
	_ = testing.MakeEmptyFakeHome(c)
	expected := filepath.Join(os.Getenv("HOME"), ".juju")
	c.Assert(config.JujuHome(), Equals, expected)
}
