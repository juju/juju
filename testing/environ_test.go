// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing_test

import (
	"os"
	"path/filepath"

	"github.com/juju/names"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/testing"
)

type fakeHomeSuite struct {
	testing.FakeJujuHomeSuite
}

var _ = gc.Suite(&fakeHomeSuite{})

func (s *fakeHomeSuite) SetUpTest(c *gc.C) {
	utils.SetHome(home)
	os.Setenv("JUJU_HOME", jujuHome)
	osenv.SetJujuHome(jujuHome)

	s.FakeJujuHomeSuite.SetUpTest(c)
}

func (s *fakeHomeSuite) TearDownTest(c *gc.C) {
	s.FakeJujuHomeSuite.TearDownTest(c)

	// Test that the environment is restored.
	c.Assert(utils.Home(), gc.Equals, jujuHome)
	c.Assert(os.Getenv("JUJU_HOME"), gc.Equals, jujuHome)
	c.Assert(osenv.JujuHome(), gc.Equals, jujuHome)
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

func (s *fakeHomeSuite) TestEnvironmentTagValid(c *gc.C) {
	asString := testing.EnvironmentTag.String()
	tag, err := names.ParseEnvironTag(asString)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tag, gc.Equals, testing.EnvironmentTag)
}

func (s *fakeHomeSuite) TestEnvironUUIDValid(c *gc.C) {
	c.Assert(utils.IsValidUUIDString(testing.EnvironmentTag.Id()), jc.IsTrue)
}
