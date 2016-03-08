// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing_test

import (
	"os"

	"github.com/juju/names"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/testing"
)

type fakeHomeSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&fakeHomeSuite{})

func (s *fakeHomeSuite) SetUpTest(c *gc.C) {
	utils.SetHome(home)
	os.Setenv("JUJU_DATA", jujuXDGDataHome)
	osenv.SetJujuXDGDataHome(jujuXDGDataHome)

	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
}

func (s *fakeHomeSuite) TearDownTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.TearDownTest(c)

	// Test that the environment is restored.
	c.Assert(utils.Home(), gc.Equals, jujuXDGDataHome)
	c.Assert(os.Getenv("JUJU_DATA"), gc.Equals, jujuXDGDataHome)
	c.Assert(osenv.JujuXDGDataHome(), gc.Equals, jujuXDGDataHome)
}

func (s *fakeHomeSuite) TestFakeHomeSetsUpJujuXDGDataHome(c *gc.C) {
	jujuDir := gitjujutesting.JujuXDGDataHomePath()
	c.Assert(jujuDir, jc.IsDirectory)
}

func (s *fakeHomeSuite) TestFakeHomeSetsConfigJujuXDGDataHome(c *gc.C) {
	s.PatchEnvironment(osenv.XDGDataHome, "")
	expected := gitjujutesting.JujuXDGDataHomePath()
	c.Assert(osenv.JujuXDGDataHome(), gc.Equals, expected)
}

func (s *fakeHomeSuite) TestModelTagValid(c *gc.C) {
	asString := testing.ModelTag.String()
	tag, err := names.ParseModelTag(asString)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tag, gc.Equals, testing.ModelTag)
}

func (s *fakeHomeSuite) TestEnvironUUIDValid(c *gc.C) {
	c.Assert(utils.IsValidUUIDString(testing.ModelTag.Id()), jc.IsTrue)
}
