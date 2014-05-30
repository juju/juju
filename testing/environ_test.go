// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing_test

import (
	"os"
	"path/filepath"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/juju/osenv"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/utils"
)

type fakeHomeSuite struct {
	testing.FakeJujuHomeSuite
}

var _ = gc.Suite(&fakeHomeSuite{})

func (s *fakeHomeSuite) SetUpTest(c *gc.C) {
	utils.SetHome("/home/eric")
	os.Setenv("JUJU_HOME", "/home/eric/juju")
	osenv.SetJujuHome("/home/eric/juju")

	s.FakeJujuHomeSuite.SetUpTest(c)
}

func (s *fakeHomeSuite) TearDownTest(c *gc.C) {
	s.FakeJujuHomeSuite.TearDownTest(c)

	// Test that the environment is restored.
	c.Assert(utils.Home(), gc.Equals, "/home/eric")
	c.Assert(os.Getenv("JUJU_HOME"), gc.Equals, "/home/eric/juju")
	c.Assert(osenv.JujuHome(), gc.Equals, "/home/eric/juju")
}

func (s *fakeHomeSuite) TestFakeHomeSetsUpHome(c *gc.C) {
	sshDir := testing.HomePath(".ssh")
	_, err := os.Stat(sshDir)
	c.Assert(err, gc.IsNil)
	jujuDir := testing.HomePath(".juju")
	_, err = os.Stat(jujuDir)
	c.Assert(err, gc.IsNil)
	envFile := testing.HomePath(".juju", "environments.yaml")
	_, err = os.Stat(envFile)
	c.Assert(err, gc.IsNil)
}

func (s *fakeHomeSuite) TestFakeHomeSetsConfigJujuHome(c *gc.C) {
	expected := filepath.Join(utils.Home(), ".juju")
	c.Assert(osenv.JujuHome(), gc.Equals, expected)
}
