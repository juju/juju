// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing_test

import (
	"os"
	"path/filepath"

	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/testing"
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

func (s *fakeHomeSuite) TestFakeHomeSetsUpJujuHome(c *gc.C) {
	jujuDir := gitjujutesting.HomePath(".juju")
	c.Assert(jujuDir, jc.IsDirectory)
	envFile := filepath.Join(jujuDir, "environments.yaml")
	c.Assert(envFile, jc.IsNonEmptyFile)
}

func (s *fakeHomeSuite) TestFakeHomeSetsConfigJujuHome(c *gc.C) {
	expected := filepath.Join(utils.Home(), ".juju")
	c.Assert(osenv.JujuHome(), gc.Equals, expected)
}
