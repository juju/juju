// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcmd_test

import (
	"os"
	"regexp"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/tc"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient"
)

type ControllerCommandSuite struct {
	testhelpers.IsolationSuite
}

var _ = tc.Suite(&ControllerCommandSuite{})

func (s *ControllerCommandSuite) TestControllerCommandNoneSpecified(c *tc.C) {
	command, err := runTestControllerCommand(c, jujuclient.NewMemStore())
	c.Assert(err, tc.ErrorIsNil)
	controllerName, err := command.ControllerName()
	c.Assert(errors.Cause(err), tc.Equals, modelcmd.ErrNoControllersDefined)
	c.Assert(controllerName, tc.Equals, "")
}

func (s *ControllerCommandSuite) TestCurrentControllerFromControllerEnvVar(c *tc.C) {
	s.PatchEnvironment("JUJU_CONTROLLER", "bar")
	store := jujuclient.NewMemStore()
	store.Controllers["bar"] = jujuclient.ControllerDetails{}
	testEnsureControllerName(c, store, "bar")
}

func (s *ControllerCommandSuite) TestCurrentControllerFromModelEnvVar(c *tc.C) {
	s.PatchEnvironment("JUJU_MODEL", "buzz:bar")
	store := jujuclient.NewMemStore()
	store.Controllers["buzz"] = jujuclient.ControllerDetails{}
	testEnsureControllerName(c, store, "buzz")
}

func (s *ControllerCommandSuite) TestCurrentControllerFromStore(c *tc.C) {
	store := jujuclient.NewMemStore()
	store.CurrentControllerName = "foo"
	store.Controllers["foo"] = jujuclient.ControllerDetails{}
	testEnsureControllerName(c, store, "foo")
}

func (s *ControllerCommandSuite) TestCurrentControllerEnvVarConflict(c *tc.C) {
	s.PatchEnvironment("JUJU_MODEL", "buzz:bar")
	s.PatchEnvironment("JUJU_CONTROLLER", "bar")
	store := jujuclient.NewMemStore()
	store.CurrentControllerName = "foo"
	store.Controllers["buzz"] = jujuclient.ControllerDetails{}
	store.Controllers["foo"] = jujuclient.ControllerDetails{}
	store.Controllers["bar"] = jujuclient.ControllerDetails{}
	command, err := runTestControllerCommand(c, store)
	c.Assert(err, tc.ErrorIsNil)
	_, err = command.ControllerName()
	c.Assert(err, tc.ErrorMatches, regexp.QuoteMeta("controller name from JUJU_MODEL (buzz) conflicts with value in JUJU_CONTROLLER (bar)"))
}

func (s *ControllerCommandSuite) TestCurrentControllerPrecedenceEnvVar(c *tc.C) {
	s.PatchEnvironment("JUJU_CONTROLLER", "bar")
	store := jujuclient.NewMemStore()
	store.CurrentControllerName = "foo"
	store.Controllers["foo"] = jujuclient.ControllerDetails{}
	store.Controllers["bar"] = jujuclient.ControllerDetails{}
	testEnsureControllerName(c, store, "bar")
}

func (s *ControllerCommandSuite) TesCurrentControllerDeterminedButNotInStore(c *tc.C) {
	s.PatchEnvironment("JUJU_CONTROLLER", "bar")
	_, err := runTestControllerCommand(c, jujuclient.NewMemStore())
	c.Assert(err, tc.ErrorMatches, "controller bar not found")
}

func (s *ControllerCommandSuite) TestControllerCommandInitExplicit(c *tc.C) {
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

func (s *ControllerCommandSuite) TestWrapWithoutFlags(c *tc.C) {
	command := new(testControllerCommand)
	wrapped := modelcmd.WrapController(command, modelcmd.WrapControllerSkipControllerFlags)
	err := cmdtesting.InitCommand(wrapped, []string{"-s", "testsys"})
	c.Assert(err, tc.ErrorMatches, "option provided but not defined: -s")
}

func (s *ControllerCommandSuite) TestInnerCommand(c *tc.C) {
	command := new(testControllerCommand)
	wrapped := modelcmd.WrapController(command)
	c.Assert(modelcmd.InnerCommand(wrapped), tc.Equals, command)
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

func testEnsureControllerName(c *tc.C, store jujuclient.ClientStore, expect string, args ...string) {
	command, err := runTestControllerCommand(c, store, args...)
	c.Assert(err, tc.ErrorIsNil)
	controllerName, err := command.ControllerName()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(controllerName, tc.Equals, expect)
}

func runTestControllerCommand(c *tc.C, store jujuclient.ClientStore, args ...string) (modelcmd.ControllerCommand, error) {
	command := modelcmd.WrapController(new(testControllerCommand))
	command.SetClientStore(store)
	_, err := cmdtesting.RunCommand(c, command, args...)
	return command, errors.Trace(err)
}

type OptionalControllerCommandSuite struct {
	testhelpers.IsolationSuite
	coretesting.JujuOSEnvSuite
}

var _ = tc.Suite(&OptionalControllerCommandSuite{})

func (s *OptionalControllerCommandSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.JujuOSEnvSuite.SetUpTest(c)
}

func (s *OptionalControllerCommandSuite) TearDownTest(c *tc.C) {
	s.IsolationSuite.TearDownTest(c)
	s.JujuOSEnvSuite.TearDownTest(c)
}

func (s *OptionalControllerCommandSuite) TestEmbedded(c *tc.C) {
	optCommand := modelcmd.OptionalControllerCommand{}
	optCommand.Embedded = true
	command := &testOptionalControllerCommand{OptionalControllerCommand: optCommand}
	_, err := cmdtesting.RunCommand(c, command, "--client")
	c.Assert(err, tc.ErrorMatches, `option provided but not defined: --client`)
}

func (s *OptionalControllerCommandSuite) assertPrompt(c *tc.C,
	store jujuclient.ClientStore,
	action string,
	userAnswer string,
	in ...string,
) (*cmd.Context, *testOptionalControllerCommand, error) {
	ctx, command, err := runOptionalControllerCommand(c, store, in...)
	c.Assert(err, tc.ErrorIsNil)
	ctx.Stdin = strings.NewReader(userAnswer)
	err = command.MaybePrompt(ctx, action)
	return ctx, command, err
}

type testData struct {
	action                  string
	userAnswer              string
	expectedPrompt          string
	expectedInfo            string
	expectedControllerName  string
	expectedClientOperation bool
	args                    []string
}

func (s *OptionalControllerCommandSuite) assertPrompted(c *tc.C, store jujuclient.ClientStore, t testData) {
	ctx, command, err := s.assertPrompt(c, store, t.action, t.userAnswer, t.args...)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(command.ControllerName, tc.Equals, t.expectedControllerName)
	c.Assert(command.Client, tc.Equals, t.expectedClientOperation)
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, t.expectedPrompt)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, t.expectedInfo)
}

func (s *OptionalControllerCommandSuite) TestPromptManyControllersNoCurrent(c *tc.C) {
	store := jujuclient.NewMemStore()
	store.Controllers = map[string]jujuclient.ControllerDetails{
		"fred": {},
		"mary": {},
	}
	s.assertPrompted(c, store, testData{
		userAnswer:     "y\n",
		expectedPrompt: "Do you ONLY want to  this client? (Y/n): \n",
		expectedInfo: "This operation can be applied to both a copy on this client and to the one on a controller.\n" +
			"No current controller was detected but there are other controllers registered: use -c or --controller to specify a controller if needed.\n",
		expectedControllerName:  "",
		expectedClientOperation: true,
	})
}

func (s *OptionalControllerCommandSuite) TestPromptNoRegisteredControllers(c *tc.C) {
	// Since there are no controllers registered on the client, the operation is
	// assumed to be desired only on the client.
	s.assertPrompted(c, jujuclient.NewMemStore(), testData{
		userAnswer:     "n\n",
		expectedPrompt: "",
		expectedInfo: "This operation can be applied to both a copy on this client and to the one on a controller.\n" +
			"No current controller was detected and there are no registered controllers on this client: either bootstrap one or register one.\n",
		expectedControllerName:  "",
		expectedClientOperation: true,
	})
}

func setupTestStore() jujuclient.ClientStore {
	store := jujuclient.NewMemStore()
	store.Controllers = map[string]jujuclient.ControllerDetails{
		"fred": {},
	}
	store.CurrentControllerName = "fred"
	return store
}

func (s *OptionalControllerCommandSuite) TestPromptDenyClientAndCurrent(c *tc.C) {
	for _, input := range []string{"q\n", "Q\n"} {
		s.assertPrompted(c, setupTestStore(), testData{
			action: "build a snowman on",
			expectedInfo: "This operation can be applied to both a copy on this client and to the one on a controller.\n" +
				"Neither client nor controller specified - nothing to do.\n",
			expectedPrompt: `
Do you want to build a snowman on:
    1. client only (--client)
    2. controller "fred" only (--controller fred)
    3. both (--client --controller fred)
Enter your choice, or type Q|q to quit: `[1:],
			userAnswer:              input,
			expectedControllerName:  "",
			expectedClientOperation: false,
		})
	}
}

func (s *OptionalControllerCommandSuite) TestPromptInvalidChoice(c *tc.C) {
	s.assertPrompted(c, setupTestStore(), testData{
		action: "build a snowman on",
		expectedInfo: "This operation can be applied to both a copy on this client and to the one on a controller.\n" +
			"Invalid choice, enter a number between 1 and 3 or quit Q|q\n" +
			"Neither client nor controller specified - nothing to do.\n",
		expectedPrompt: `
Do you want to build a snowman on:
    1. client only (--client)
    2. controller "fred" only (--controller fred)
    3. both (--client --controller fred)
Enter your choice, or type Q|q to quit: `[1:],
		userAnswer:              "5\nq\n",
		expectedControllerName:  "",
		expectedClientOperation: false,
	})
}

func (s *OptionalControllerCommandSuite) TestPromptConfirmClient(c *tc.C) {
	s.assertPrompted(c, setupTestStore(), testData{
		action:       "build a snowman on",
		expectedInfo: "This operation can be applied to both a copy on this client and to the one on a controller.\n",
		expectedPrompt: `
Do you want to build a snowman on:
    1. client only (--client)
    2. controller "fred" only (--controller fred)
    3. both (--client --controller fred)
Enter your choice, or type Q|q to quit: `[1:],
		userAnswer:              "1\n",
		expectedControllerName:  "",
		expectedClientOperation: true,
	})
}

func (s *OptionalControllerCommandSuite) TestPromptConfirmController(c *tc.C) {
	s.assertPrompted(c, setupTestStore(), testData{
		action:       "build a snowman on",
		expectedInfo: "This operation can be applied to both a copy on this client and to the one on a controller.\n",
		expectedPrompt: `
Do you want to build a snowman on:
    1. client only (--client)
    2. controller "fred" only (--controller fred)
    3. both (--client --controller fred)
Enter your choice, or type Q|q to quit: `[1:],
		userAnswer:              "2\n",
		expectedControllerName:  "fred",
		expectedClientOperation: false,
	})
}

func (s *OptionalControllerCommandSuite) TestPromptConfirmBoth(c *tc.C) {
	s.assertPrompted(c, setupTestStore(), testData{
		action:       "build a snowman on",
		expectedInfo: "This operation can be applied to both a copy on this client and to the one on a controller.\n",
		expectedPrompt: `
Do you want to build a snowman on:
    1. client only (--client)
    2. controller "fred" only (--controller fred)
    3. both (--client --controller fred)
Enter your choice, or type Q|q to quit: `[1:],
		userAnswer:              "3\n",
		expectedControllerName:  "fred",
		expectedClientOperation: true,
	})
}

func (s *OptionalControllerCommandSuite) assertNoPromptForReadOnlyCommands(c *tc.C, store jujuclient.ClientStore, expectedErr, expectedOut, expectedController string) {
	command := &testOptionalControllerCommand{
		OptionalControllerCommand: modelcmd.OptionalControllerCommand{Store: store, ReadOnly: true},
	}
	ctx, err := cmdtesting.RunCommand(c, command)
	c.Assert(err, tc.ErrorIsNil)
	err = command.MaybePrompt(ctx, "add a cloud")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, expectedOut)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, expectedErr)
	c.Assert(command.ControllerName, tc.Equals, expectedController)
	c.Assert(command.Client, tc.IsTrue)

}

func (s *OptionalControllerCommandSuite) TestNoPromptForReadOnlyNoCurrentController(c *tc.C) {
	s.assertNoPromptForReadOnlyCommands(c, jujuclient.NewMemStore(), "", "", "")
}

func (s *OptionalControllerCommandSuite) TestNoPromptForReadOnlyWithCurrentController(c *tc.C) {
	s.assertNoPromptForReadOnlyCommands(c, setupTestStore(), "", "", "fred")
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

func runOptionalControllerCommand(c *tc.C, store jujuclient.ClientStore, args ...string) (*cmd.Context, *testOptionalControllerCommand, error) {
	optCommand := modelcmd.OptionalControllerCommand{Store: store}
	command := &testOptionalControllerCommand{OptionalControllerCommand: optCommand}
	ctx, err := cmdtesting.RunCommand(c, command, args...)
	return ctx, command, errors.Trace(err)
}
