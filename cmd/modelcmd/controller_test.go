// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcmd_test

import (
	"os"
	"regexp"
	"strings"

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
	command, err := runTestControllerCommand(c, jujuclient.NewMemStore())
	c.Assert(err, jc.ErrorIsNil)
	controllerName, err := command.ControllerName()
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
	command, err := runTestControllerCommand(c, store)
	c.Assert(err, jc.ErrorIsNil)
	_, err = command.ControllerName()
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
	command := new(testControllerCommand)
	wrapped := modelcmd.WrapController(command, modelcmd.WrapControllerSkipControllerFlags)
	err := cmdtesting.InitCommand(wrapped, []string{"-s", "testsys"})
	c.Assert(err, gc.ErrorMatches, "option provided but not defined: -s")
}

func (s *ControllerCommandSuite) TestInnerCommand(c *gc.C) {
	command := new(testControllerCommand)
	wrapped := modelcmd.WrapController(command)
	c.Assert(modelcmd.InnerCommand(wrapped), gc.Equals, command)
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
	command, err := runTestControllerCommand(c, store, args...)
	c.Assert(err, jc.ErrorIsNil)
	controllerName, err := command.ControllerName()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllerName, gc.Equals, expect)
}

func runTestControllerCommand(c *gc.C, store jujuclient.ClientStore, args ...string) (modelcmd.ControllerCommand, error) {
	command := modelcmd.WrapController(new(testControllerCommand))
	command.SetClientStore(store)
	_, err := cmdtesting.RunCommand(c, command, args...)
	return command, errors.Trace(err)
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
	command, err := runTestOptionalControllerCommand(c, "", jujuclient.NewMemStore())
	c.Assert(err, jc.ErrorIsNil)
	_, err = command.ControllerNameFromArg()
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
	command, err := runTestOptionalControllerCommand(c, "", store)
	c.Assert(err, jc.ErrorIsNil)
	controllerName, err := command.ControllerNameFromArg()
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
	command, err := runTestOptionalControllerCommand(c, "", store)
	c.Assert(err, jc.ErrorIsNil)
	controllerName, err := command.ControllerNameFromArg()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllerName, gc.Equals, "mary")
}

func (s *OptionalControllerCommandSuite) TestControllerCommandLocal(c *gc.C) {
	store := jujuclient.NewMemStore()
	store.Controllers = map[string]jujuclient.ControllerDetails{
		"fred": {},
	}
	store.CurrentControllerName = "fred"
	command, err := runTestOptionalControllerCommand(c, "", store, "--local")
	c.Assert(err, jc.ErrorIsNil)
	controllerName, err := command.ControllerNameFromArg()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllerName, gc.Equals, "")
}

func (s *OptionalControllerCommandSuite) TestControllerCommandNoFlag(c *gc.C) {
	store := jujuclient.NewMemStore()
	store.Controllers = map[string]jujuclient.ControllerDetails{
		"fred": {},
	}
	store.CurrentControllerName = "fred"
	command, err := runTestOptionalControllerCommand(c, "multi-cloud", store)
	c.Assert(err, jc.ErrorIsNil)
	controllerName, err := command.ControllerNameFromArg()
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
	command, err := runTestOptionalControllerCommand(c, "multi-cloud", store)
	c.Assert(err, jc.ErrorIsNil)
	controllerName, err := command.ControllerNameFromArg()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllerName, gc.Equals, "fred")
}

func (s *OptionalControllerCommandSuite) assertDetectCurrentController(c *gc.C,
	store jujuclient.ClientStore,
	expectedControllerName string,
	in ...string,
) {
	ctx, command, err := runOptionalControllerCommand(c, "", store, in...)
	c.Assert(err, jc.ErrorIsNil)
	controllerName, err := command.MaybePromptCurrentController(ctx, "")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllerName, gc.Equals, expectedControllerName)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
}

func (s *OptionalControllerCommandSuite) TestDetectCurrentControllerNoControllers(c *gc.C) {
	s.assertDetectCurrentController(c, jujuclient.NewMemStore(), "")
}

func (s *OptionalControllerCommandSuite) TestDetectCurrentControllerNoCurrentController(c *gc.C) {
	store := jujuclient.NewMemStore()
	store.Controllers = map[string]jujuclient.ControllerDetails{
		"fred": {},
	}
	s.assertDetectCurrentController(c, store, "")
}

func (s *OptionalControllerCommandSuite) TestDetectCurrentControllerSkipPrompt(c *gc.C) {
	store := jujuclient.NewMemStore()
	store.Controllers = map[string]jujuclient.ControllerDetails{
		"fred": {},
	}
	store.CurrentControllerName = "fred"
	s.assertDetectCurrentController(c, store, "fred", "--skipPrompt")
}

func (s *OptionalControllerCommandSuite) assertDetectCurrentControllerPrompt(c *gc.C, userAnswer, expectedControllerName string) {
	store := jujuclient.NewMemStore()
	store.Controllers = map[string]jujuclient.ControllerDetails{
		"fred": {},
	}
	store.CurrentControllerName = "fred"
	ctx, command, err := runOptionalControllerCommand(c, "", store)
	c.Assert(err, jc.ErrorIsNil)
	ctx.Stdin = strings.NewReader(userAnswer)
	controllerName, err := command.MaybePromptCurrentController(ctx, "test on")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllerName, gc.Equals, expectedControllerName)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "Do you want to test on current controller \"fred\".? (Y/n): \n")
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
}

func (s *OptionalControllerCommandSuite) TestDetectCurrentControllerPromptConfirm(c *gc.C) {
	s.assertDetectCurrentControllerPrompt(c, "y", "fred")
}

func (s *OptionalControllerCommandSuite) TestDetectCurrentControllerPromptDeny(c *gc.C) {
	s.assertDetectCurrentControllerPrompt(c, "n", "")
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
	_, command, err := runOptionalControllerCommand(c, enabledFlag, store, args...)
	return command, errors.Trace(err)
}

func runOptionalControllerCommand(c *gc.C, enabledFlag string, store jujuclient.ClientStore, args ...string) (*cmd.Context, *testOptionalControllerCommand, error) {
	optCommand := modelcmd.OptionalControllerCommand{Store: store}
	if enabledFlag != "" {
		optCommand.EnabledFlag = enabledFlag
	}
	command := &testOptionalControllerCommand{OptionalControllerCommand: optCommand}
	ctx, err := cmdtesting.RunCommand(c, command, args...)
	return ctx, command, errors.Trace(err)
}
