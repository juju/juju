// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package envcmd_test

import (
	"os"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/testing"
)

type ControllerCommandSuite struct {
	testing.FakeJujuHomeSuite
}

var _ = gc.Suite(&ControllerCommandSuite{})

func (s *ControllerCommandSuite) TestControllerCommandInitMultipleConfigs(c *gc.C) {
	// The environments.yaml file is ignored for controller commands.
	testing.WriteEnvironments(c, testing.MultipleEnvConfig)
	_, err := initTestControllerCommand(c)
	c.Assert(err, gc.ErrorMatches, "no controller specified")
}

func (s *ControllerCommandSuite) TestControllerCommandInitNoEnvFile(c *gc.C) {
	// Since we ignore the environments.yaml file, we don't care if it isn't
	// there.
	envPath := gitjujutesting.HomePath(".juju", "environments.yaml")
	err := os.Remove(envPath)
	_, err = initTestControllerCommand(c)
	c.Assert(err, gc.ErrorMatches, "no controller specified")
}

func (s *ControllerCommandSuite) TestControllerCommandInitSystemFile(c *gc.C) {
	// If there is a current-controller file, use that.
	err := envcmd.WriteCurrentController("fubar")
	c.Assert(err, jc.ErrorIsNil)
	testEnsureControllerName(c, "fubar")
}
func (s *ControllerCommandSuite) TestControllerCommandInitEnvFile(c *gc.C) {
	// If there is a current-environment file, use that.
	err := envcmd.WriteCurrentEnvironment("fubar")
	c.Assert(err, jc.ErrorIsNil)
	testEnsureControllerName(c, "fubar")
}

func (s *ControllerCommandSuite) TestControllerCommandInitExplicit(c *gc.C) {
	// Take controller name from command line arg, and it trumps the current-
	// controller file.
	err := envcmd.WriteCurrentController("fubar")
	c.Assert(err, jc.ErrorIsNil)
	testEnsureControllerName(c, "explicit", "-c", "explicit")
	testEnsureControllerName(c, "explicit", "--controller", "explicit")
}

func (s *ControllerCommandSuite) TestWrapWithoutFlags(c *gc.C) {
	cmd := new(testControllerCommand)
	wrapped := envcmd.WrapController(cmd, envcmd.ControllerSkipFlags)
	err := cmdtesting.InitCommand(wrapped, []string{"-s", "testsys"})
	c.Assert(err, gc.ErrorMatches, "flag provided but not defined: -s")
}

type testControllerCommand struct {
	envcmd.ControllerCommandBase
}

func (c *testControllerCommand) Info() *cmd.Info {
	panic("should not be called")
}

func (c *testControllerCommand) Run(ctx *cmd.Context) error {
	panic("should not be called")
}

func initTestControllerCommand(c *gc.C, args ...string) (*testControllerCommand, error) {
	cmd := new(testControllerCommand)
	wrapped := envcmd.WrapController(cmd)
	return cmd, cmdtesting.InitCommand(wrapped, args)
}

func testEnsureControllerName(c *gc.C, expect string, args ...string) {
	cmd, err := initTestControllerCommand(c, args...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmd.ControllerName(), gc.Equals, expect)
}
