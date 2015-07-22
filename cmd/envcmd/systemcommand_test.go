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

type SystemCommandSuite struct {
	testing.FakeJujuHomeSuite
}

var _ = gc.Suite(&SystemCommandSuite{})

func (s *SystemCommandSuite) TestSystemCommandInitMultipleConfigs(c *gc.C) {
	// The environments.yaml file is ignored for system commands.
	testing.WriteEnvironments(c, testing.MultipleEnvConfig)
	_, err := initTestSystemCommand(c)
	c.Assert(err, gc.ErrorMatches, "no system specified")
}

func (s *SystemCommandSuite) TestSystemCommandInitNoEnvFile(c *gc.C) {
	// Since we ignore the environments.yaml file, we don't care if it isn't
	// there.
	envPath := gitjujutesting.HomePath(".juju", "environments.yaml")
	err := os.Remove(envPath)
	_, err = initTestSystemCommand(c)
	c.Assert(err, gc.ErrorMatches, "no system specified")
}

func (s *SystemCommandSuite) TestSystemCommandInitSystemFile(c *gc.C) {
	// If there is a current-system file, use that.
	err := envcmd.WriteCurrentSystem("fubar")
	c.Assert(err, jc.ErrorIsNil)
	testEnsureSystemName(c, "fubar")
}
func (s *SystemCommandSuite) TestSystemCommandInitEnvFile(c *gc.C) {
	// If there is a current-environment file, use that.
	err := envcmd.WriteCurrentEnvironment("fubar")
	c.Assert(err, jc.ErrorIsNil)
	testEnsureSystemName(c, "fubar")
}

func (s *SystemCommandSuite) TestSystemCommandInitExplicit(c *gc.C) {
	// Take system name from command line arg, and it trumps the current-
	// system file.
	err := envcmd.WriteCurrentSystem("fubar")
	c.Assert(err, jc.ErrorIsNil)
	testEnsureSystemName(c, "explicit", "-s", "explicit")
	testEnsureSystemName(c, "explicit", "--system", "explicit")
}

type testSystemCommand struct {
	envcmd.SysCommandBase
}

func (c *testSystemCommand) Info() *cmd.Info {
	panic("should not be called")
}

func (c *testSystemCommand) Run(ctx *cmd.Context) error {
	panic("should not be called")
}

func initTestSystemCommand(c *gc.C, args ...string) (*testSystemCommand, error) {
	cmd := new(testSystemCommand)
	wrapped := envcmd.WrapSystem(cmd)
	return cmd, cmdtesting.InitCommand(wrapped, args)
}

func testEnsureSystemName(c *gc.C, expect string, args ...string) {
	cmd, err := initTestSystemCommand(c, args...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmd.SystemName(), gc.Equals, expect)
}
