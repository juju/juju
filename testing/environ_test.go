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
	testing.FakeJujuDataSuite
}

var _ = gc.Suite(&fakeHomeSuite{})

func (s *fakeHomeSuite) SetUpTest(c *gc.C) {
	utils.SetHome(home)
	os.Setenv("JUJU_DATA", jujuHome)
	osenv.SetJujuData(jujuHome)

	s.FakeJujuDataSuite.SetUpTest(c)
}

func (s *fakeHomeSuite) TearDownTest(c *gc.C) {
	s.FakeJujuDataSuite.TearDownTest(c)

	// Test that the environment is restored.
	c.Assert(utils.Home(), gc.Equals, jujuHome)
	c.Assert(os.Getenv("JUJU_DATA"), gc.Equals, jujuHome)
	c.Assert(osenv.JujuData(), gc.Equals, jujuHome)
}

func (s *fakeHomeSuite) TestFakeHomeSetsUpJujuData(c *gc.C) {
	jujuDir := gitjujutesting.JujuDataPath()
	c.Assert(jujuDir, jc.IsDirectory)
	envFile := filepath.Join(jujuDir, "environments.yaml")
	c.Assert(envFile, jc.IsNonEmptyFile)
}

func (s *fakeHomeSuite) TestFakeHomeSetsConfigJujuData(c *gc.C) {
	s.PatchEnvironment(osenv.XDGDataHome, "")
	expected := gitjujutesting.JujuDataPath()
	c.Assert(osenv.JujuData(), gc.Equals, expected)
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
