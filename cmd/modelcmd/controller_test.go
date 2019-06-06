// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcmd_test

import (
	"os"
	"regexp"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient"
	coretesting "github.com/juju/juju/testing"
)

type ControllerCommandSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ControllerCommandSuite{})

func (s *ControllerCommandSuite) TestControllerCommandNoneSpecified(c *gc.C) {
	cmd, err := runTestControllerCommand(c, jujuclient.NewMemStore())
	c.Assert(err, jc.ErrorIsNil)
	controllerName, err := cmd.ControllerName()
	c.Assert(errors.Cause(err), gc.Equals, modelcmd.ErrNoControllersDefined)
	c.Assert(controllerName, gc.Equals, "")
}

func (s *ControllerCommandSuite) TestCurrentControllerFromControllerEnvVar(c *gc.C) {
	s.PatchEnvironment("JUJU_CONTROLLER", "bar")
	store := jujuclient.NewMemStore()
	store.Controllers["bar"] = jujuclient.ControllerDetails{}
	testEnsureControllerName(c, store, "bar")
}

func (s *ControllerCommandSuite) TestCurrentControllerFromModelEnvVar(c *gc.C) {
	s.PatchEnvironment("JUJU_MODEL", "buzz:bar")
	store := jujuclient.NewMemStore()
	store.Controllers["buzz"] = jujuclient.ControllerDetails{}
	testEnsureControllerName(c, store, "buzz")
}

func (s *ControllerCommandSuite) TestCurrentControllerFromStore(c *gc.C) {
	store := jujuclient.NewMemStore()
	store.CurrentControllerName = "foo"
	store.Controllers["foo"] = jujuclient.ControllerDetails{}
	testEnsureControllerName(c, store, "foo")
}

func (s *ControllerCommandSuite) TestCurrentControllerEnvVarConflict(c *gc.C) {
	s.PatchEnvironment("JUJU_MODEL", "buzz:bar")
	s.PatchEnvironment("JUJU_CONTROLLER", "bar")
	store := jujuclient.NewMemStore()
	store.CurrentControllerName = "foo"
	store.Controllers["buzz"] = jujuclient.ControllerDetails{}
	store.Controllers["foo"] = jujuclient.ControllerDetails{}
	store.Controllers["bar"] = jujuclient.ControllerDetails{}
	cmd, err := runTestControllerCommand(c, store)
	c.Assert(err, jc.ErrorIsNil)
	_, err = cmd.ControllerName()
	c.Assert(err, gc.ErrorMatches, regexp.QuoteMeta("controller name from JUJU_MODEL (buzz) conflicts with value in JUJU_CONTROLLER (bar)"))
}

func (s *ControllerCommandSuite) TestCurrentControllerPrecedenceEnvVar(c *gc.C) {
	s.PatchEnvironment("JUJU_CONTROLLER", "bar")
	store := jujuclient.NewMemStore()
	store.CurrentControllerName = "foo"
	store.Controllers["foo"] = jujuclient.ControllerDetails{}
	store.Controllers["bar"] = jujuclient.ControllerDetails{}
	testEnsureControllerName(c, store, "bar")
}

func (s *ControllerCommandSuite) TesCurrentControllerDeterminedButNotInStore(c *gc.C) {
	s.PatchEnvironment("JUJU_CONTROLLER", "bar")
	_, err := runTestControllerCommand(c, jujuclient.NewMemStore())
	c.Assert(err, gc.ErrorMatches, "controller bar not found")
}

func (s *ControllerCommandSuite) TestControllerCommandInitExplicit(c *gc.C) {
	// Take controller name from command line arg, and it trumps the current-
	// controller file.
	store := jujuclient.NewMemStore()
	store.CurrentControllerName = "foo"
	store.Accounts["explicit"] = jujuclient.AccountDetails{
		User: "bar",
	}
	store.Controllers["explicit"] = jujuclient.ControllerDetails{}
	testEnsureControllerName(c, store, "explicit", "-c", "explicit")
	testEnsureControllerName(c, store, "explicit", "--controller", "explicit")
	os.Setenv(osenv.JujuControllerEnvKey, "explicit")
	testEnsureControllerName(c, store, "explicit")
}

func (s *ControllerCommandSuite) TestWrapWithoutFlags(c *gc.C) {
	cmd := new(testControllerCommand)
	wrapped := modelcmd.WrapController(cmd, modelcmd.WrapControllerSkipControllerFlags)
	err := cmdtesting.InitCommand(wrapped, []string{"-s", "testsys"})
	c.Assert(err, gc.ErrorMatches, "option provided but not defined: -s")
}

func (s *ControllerCommandSuite) TestInnerCommand(c *gc.C) {
	cmd := new(testControllerCommand)
	wrapped := modelcmd.WrapController(cmd)
	c.Assert(modelcmd.InnerCommand(wrapped), gc.Equals, cmd)
}

type testControllerCommand struct {
	modelcmd.ControllerCommandBase
}

func (c *testControllerCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:        "testControllerCommand",
		FlagKnownAs: "option",
	})
}

func (c *testControllerCommand) Run(ctx *cmd.Context) error {
	return nil
}

func testEnsureControllerName(c *gc.C, store jujuclient.ClientStore, expect string, args ...string) {
	cmd, err := runTestControllerCommand(c, store, args...)
	c.Assert(err, jc.ErrorIsNil)
	controllerName, err := cmd.ControllerName()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllerName, gc.Equals, expect)
}

func runTestControllerCommand(c *gc.C, store jujuclient.ClientStore, args ...string) (modelcmd.ControllerCommand, error) {
	cmd := modelcmd.WrapController(new(testControllerCommand))
	cmd.SetClientStore(store)
	_, err := cmdtesting.RunCommand(c, cmd, args...)
	return cmd, errors.Trace(err)
}

type OptionalControllerCommandSuite struct {
	testing.IsolationSuite
	coretesting.JujuOSEnvSuite
}

var _ = gc.Suite(&OptionalControllerCommandSuite{})

func (s *OptionalControllerCommandSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.JujuOSEnvSuite.SetUpTest(c)
}

func (s *OptionalControllerCommandSuite) TearDownTest(c *gc.C) {
	s.IsolationSuite.TearDownTest(c)
	s.JujuOSEnvSuite.TearDownTest(c)
}

func (s *OptionalControllerCommandSuite) TestControllerCommandNoneRunning(c *gc.C) {
	cmd, err := runTestOptionalControllerCommand(c, "", jujuclient.NewMemStore())
	c.Assert(err, jc.ErrorIsNil)
	_, err = cmd.ControllerNameFromArg()
	c.Assert(err, gc.NotNil)
	c.Assert(err.Error(), gc.Equals, `
No controllers registered.

Please either create a new controller using "juju bootstrap" or connect to
another controller that you have been given access to using "juju register".
`[1:])
}

func (s *OptionalControllerCommandSuite) TestControllerCommandCurrent(c *gc.C) {
	store := jujuclient.NewMemStore()
	store.Controllers = map[string]jujuclient.ControllerDetails{
		"fred": {},
	}
	store.CurrentControllerName = "fred"
	cmd, err := runTestOptionalControllerCommand(c, "", store)
	c.Assert(err, jc.ErrorIsNil)
	controllerName, err := cmd.ControllerNameFromArg()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllerName, gc.Equals, "fred")
}

func (s *OptionalControllerCommandSuite) TestControllerCommandCurrentFromEnv(c *gc.C) {
	s.PatchEnvironment("JUJU_CONTROLLER", "mary")
	store := jujuclient.NewMemStore()
	store.Controllers = map[string]jujuclient.ControllerDetails{
		"fred": {},
		"mary": {},
	}
	store.CurrentControllerName = "fred"
	cmd, err := runTestOptionalControllerCommand(c, "", store)
	c.Assert(err, jc.ErrorIsNil)
	controllerName, err := cmd.ControllerNameFromArg()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllerName, gc.Equals, "mary")
}

func (s *OptionalControllerCommandSuite) TestControllerCommandLocal(c *gc.C) {
	store := jujuclient.NewMemStore()
	store.Controllers = map[string]jujuclient.ControllerDetails{
		"fred": {},
	}
	store.CurrentControllerName = "fred"
	cmd, err := runTestOptionalControllerCommand(c, "", store, "--local")
	c.Assert(err, jc.ErrorIsNil)
	controllerName, err := cmd.ControllerNameFromArg()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllerName, gc.Equals, "")
}

func (s *OptionalControllerCommandSuite) TestControllerCommandNoFlag(c *gc.C) {
	store := jujuclient.NewMemStore()
	store.Controllers = map[string]jujuclient.ControllerDetails{
		"fred": {},
	}
	store.CurrentControllerName = "fred"
	cmd, err := runTestOptionalControllerCommand(c, "multi-cloud", store)
	c.Assert(err, jc.ErrorIsNil)
	controllerName, err := cmd.ControllerNameFromArg()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllerName, gc.Equals, "")
}

func (s *OptionalControllerCommandSuite) TestControllerCommandWithFlag(c *gc.C) {
	s.SetFeatureFlags("multi-cloud")
	store := jujuclient.NewMemStore()
	store.Controllers = map[string]jujuclient.ControllerDetails{
		"fred": {},
	}
	store.CurrentControllerName = "fred"
	cmd, err := runTestOptionalControllerCommand(c, "multi-cloud", store)
	c.Assert(err, jc.ErrorIsNil)
	controllerName, err := cmd.ControllerNameFromArg()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllerName, gc.Equals, "fred")
}

type testOptionalControllerCommand struct {
	modelcmd.OptionalControllerCommand
}

func (c *testOptionalControllerCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:        "testOptionalControllerCommand",
		FlagKnownAs: "option",
	})
}

func (c *testOptionalControllerCommand) Run(ctx *cmd.Context) error {
	return nil
}

func runTestOptionalControllerCommand(c *gc.C, enabledFlag string, store jujuclient.ClientStore, args ...string) (*testOptionalControllerCommand, error) {
	cmd := &testOptionalControllerCommand{
		OptionalControllerCommand: modelcmd.OptionalControllerCommand{
			Store:       store,
			EnabledFlag: enabledFlag,
		},
	}
	_, err := cmdtesting.RunCommand(c, cmd, args...)
	return cmd, errors.Trace(err)
}
