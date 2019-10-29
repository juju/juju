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

func (s *OptionalControllerCommandSuite) TestControllerCommandLocal(c *gc.C) {
	store := jujuclient.NewMemStore()
	store.Controllers = map[string]jujuclient.ControllerDetails{
		"fred": {},
	}
	store.CurrentControllerName = "fred"
	command, err := runTestOptionalControllerCommand(c, store, "--local")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(command.ControllerName, gc.Equals, "")
}

func (s *OptionalControllerCommandSuite) assertPrompt(c *gc.C,
	store jujuclient.ClientStore,
	userAnswer string,
	in ...string,
) (*cmd.Context, *testOptionalControllerCommand, error) {
	ctx, command, err := runOptionalControllerCommand(c, store, in...)
	c.Assert(err, jc.ErrorIsNil)
	ctx.Stdin = strings.NewReader(userAnswer)
	err = command.MaybePrompt(ctx, "")
	return ctx, command, err
}
func (s *OptionalControllerCommandSuite) assertDetectCurrentController(c *gc.C,
	store jujuclient.ClientStore,
	userAnswer string,
	expectedControllerName string,
	expectedClientOperation bool,
	in ...string,
) {
	ctx, command, err := s.assertPrompt(c, store, userAnswer, in...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(command.ControllerName, gc.Equals, expectedControllerName)
	c.Assert(command.Client, gc.Equals, expectedClientOperation)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `
This operation can be applied to both a copy on this client and a controller of your choice.
Do you want to  this client? (Y/n): 
Do you want to  a controller? (Y/n): 
Controller Names
  fred

Select controller name [fred]: 
`[1:])
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
}

func (s *OptionalControllerCommandSuite) TestPromptNoControllers(c *gc.C) {
	ctx, command, err := s.assertPrompt(c, jujuclient.NewMemStore(), "n\ny\n")
	c.Assert(err, gc.ErrorMatches, "registered controllers on this client not found")
	c.Assert(command.ControllerName, gc.Equals, "")
	c.Assert(command.Client, gc.Equals, false)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `
This operation can be applied to both a copy on this client and a controller of your choice.
Do you want to  this client? (Y/n): 
Do you want to  a controller? (Y/n): 
`[1:])
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
}

func (s *OptionalControllerCommandSuite) TestDetectCurrentControllerNoCurrentController(c *gc.C) {
	store := jujuclient.NewMemStore()
	store.Controllers = map[string]jujuclient.ControllerDetails{
		"fred": {},
	}
	s.assertDetectCurrentController(c, store, "n\ny\nfred\n", "fred", false)
}

func (s *OptionalControllerCommandSuite) assertDetectCurrentControllerPrompt(c *gc.C,
	userAnswer, expectedControllerName string,
	expectedClientOperation bool,
	expectedOut string,
) {
	store := jujuclient.NewMemStore()
	store.Controllers = map[string]jujuclient.ControllerDetails{
		"fred": {},
	}
	store.CurrentControllerName = "fred"
	ctx, command, err := runOptionalControllerCommand(c, store)
	c.Assert(err, jc.ErrorIsNil)
	ctx.Stdin = strings.NewReader(userAnswer)
	err = command.MaybePrompt(ctx, "test on")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(command.ControllerName, gc.Equals, expectedControllerName)
	c.Assert(command.Client, gc.Equals, expectedClientOperation)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, expectedOut)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
}

var testFullOutput = `
This operation can be applied to both a copy on this client and a controller of your choice.
Do you want to test on this client? (Y/n): 
Do you want to test on a controller? (Y/n): 
Controller Names
  fred

Select controller name [fred]: 
`[1:]

var testShortOutput = `
This operation can be applied to both a copy on this client and a controller of your choice.
Do you want to test on this client? (Y/n): 
Do you want to test on a controller? (Y/n): 
`[1:]

func (s *OptionalControllerCommandSuite) TestPrompt(c *gc.C) {
	s.assertDetectCurrentControllerPrompt(c, "y\ny\n\n", "fred", true, testFullOutput)
}

func (s *OptionalControllerCommandSuite) TestPromptDeny(c *gc.C) {
	s.assertDetectCurrentControllerPrompt(c, "n\nn\n", "", false, testShortOutput)
}

func (s *OptionalControllerCommandSuite) TestPromptUseNonDefaultController(c *gc.C) {
	s.assertDetectCurrentControllerPrompt(c, "n\ny\nfred\n", "fred", false, testFullOutput)
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

func runTestOptionalControllerCommand(c *gc.C, store jujuclient.ClientStore, args ...string) (*testOptionalControllerCommand, error) {
	_, command, err := runOptionalControllerCommand(c, store, args...)
	return command, errors.Trace(err)
}

func runOptionalControllerCommand(c *gc.C, store jujuclient.ClientStore, args ...string) (*cmd.Context, *testOptionalControllerCommand, error) {
	optCommand := modelcmd.OptionalControllerCommand{Store: store}
	command := &testOptionalControllerCommand{OptionalControllerCommand: optCommand}
	ctx, err := cmdtesting.RunCommand(c, command, args...)
	return ctx, command, errors.Trace(err)
}
