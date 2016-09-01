// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcmd_test

import (
	"fmt"
	"os"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	apitesting "github.com/juju/juju/api/testing"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/testing"
)

type ModelCommandSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	store *jujuclienttesting.MemStore
}

func (s *ModelCommandSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.PatchEnvironment("JUJU_CLI_VERSION", "")

	s.store = jujuclienttesting.NewMemStore()
	s.store.CurrentControllerName = "foo"
	s.store.Controllers["foo"] = jujuclient.ControllerDetails{}
	s.store.Accounts["foo"] = jujuclient.AccountDetails{
		User: "bar@local", Password: "hunter2",
	}
}

var _ = gc.Suite(&ModelCommandSuite{})

func (s *ModelCommandSuite) TestGetCurrentModelNothingSet(c *gc.C) {
	s.store.CurrentControllerName = ""
	env, err := modelcmd.GetCurrentModel(s.store)
	c.Assert(env, gc.Equals, "")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ModelCommandSuite) TestGetCurrentModelCurrentControllerNoCurrentModel(c *gc.C) {
	env, err := modelcmd.GetCurrentModel(s.store)
	c.Assert(env, gc.Equals, "foo:")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ModelCommandSuite) TestGetCurrentModelCurrentControllerModel(c *gc.C) {
	err := s.store.UpdateModel("foo", "admin@local/mymodel", jujuclient.ModelDetails{"uuid"})
	c.Assert(err, jc.ErrorIsNil)
	err = s.store.SetCurrentModel("foo", "admin@local/mymodel")
	c.Assert(err, jc.ErrorIsNil)

	env, err := modelcmd.GetCurrentModel(s.store)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env, gc.Equals, "foo:admin@local/mymodel")
}

func (s *ModelCommandSuite) TestGetCurrentModelJujuEnvSet(c *gc.C) {
	os.Setenv(osenv.JujuModelEnvKey, "admin@local/magic")
	env, err := modelcmd.GetCurrentModel(s.store)
	c.Assert(env, gc.Equals, "admin@local/magic")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ModelCommandSuite) TestGetCurrentModelBothSet(c *gc.C) {
	os.Setenv(osenv.JujuModelEnvKey, "admin@local/magic")

	err := s.store.UpdateModel("foo", "admin@local/mymodel", jujuclient.ModelDetails{"uuid"})
	c.Assert(err, jc.ErrorIsNil)
	err = s.store.SetCurrentModel("foo", "admin@local/mymodel")
	c.Assert(err, jc.ErrorIsNil)

	env, err := modelcmd.GetCurrentModel(s.store)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env, gc.Equals, "admin@local/magic")
}

func (s *ModelCommandSuite) TestModelCommandInitExplicit(c *gc.C) {
	// Take model name from command line arg.
	s.testEnsureModelName(c, "explicit", "-m", "explicit")
}

func (s *ModelCommandSuite) TestModelCommandInitExplicitLongForm(c *gc.C) {
	// Take model name from command line arg.
	s.testEnsureModelName(c, "explicit", "--model", "explicit")
}

func (s *ModelCommandSuite) TestModelCommandInitEnvFile(c *gc.C) {
	err := s.store.UpdateModel("foo", "admin@local/mymodel", jujuclient.ModelDetails{"uuid"})
	c.Assert(err, jc.ErrorIsNil)
	err = s.store.SetCurrentModel("foo", "admin@local/mymodel")
	c.Assert(err, jc.ErrorIsNil)
	s.testEnsureModelName(c, "admin@local/mymodel")
}

func (s *ModelCommandSuite) TestBootstrapContext(c *gc.C) {
	ctx := modelcmd.BootstrapContext(&cmd.Context{})
	c.Assert(ctx.ShouldVerifyCredentials(), jc.IsTrue)
}

func (s *ModelCommandSuite) TestBootstrapContextNoVerify(c *gc.C) {
	ctx := modelcmd.BootstrapContextNoVerify(&cmd.Context{})
	c.Assert(ctx.ShouldVerifyCredentials(), jc.IsFalse)
}

func (s *ModelCommandSuite) TestWrapWithoutFlags(c *gc.C) {
	cmd := new(testCommand)
	wrapped := modelcmd.Wrap(cmd, modelcmd.WrapSkipModelFlags)
	args := []string{"-m", "testenv"}
	err := cmdtesting.InitCommand(wrapped, args)
	// 1st position is always the flag
	msg := fmt.Sprintf("flag provided but not defined: %v", args[0])
	c.Assert(err, gc.ErrorMatches, msg)
}

func (*ModelCommandSuite) TestSplitModelName(c *gc.C) {
	assert := func(in, controller, model string) {
		outController, outModel := modelcmd.SplitModelName(in)
		c.Assert(outController, gc.Equals, controller)
		c.Assert(outModel, gc.Equals, model)
	}
	assert("model", "", "model")
	assert("ctrl:model", "ctrl", "model")
	assert("ctrl:", "ctrl", "")
	assert(":model", "", "model")
}

func (*ModelCommandSuite) TestJoinModelName(c *gc.C) {
	assert := func(controller, model, expect string) {
		out := modelcmd.JoinModelName(controller, model)
		c.Assert(out, gc.Equals, expect)
	}
	assert("ctrl", "", "ctrl:")
	assert("", "model", ":model")
	assert("ctrl", "model", "ctrl:model")
}

func (s *ModelCommandSuite) testEnsureModelName(c *gc.C, expect string, args ...string) {
	cmd, err := initTestCommand(c, s.store, args...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmd.ConnectionName(), gc.Equals, expect)
}

type testCommand struct {
	modelcmd.ModelCommandBase
}

func (c *testCommand) Info() *cmd.Info {
	panic("should not be called")
}

func (c *testCommand) Run(ctx *cmd.Context) error {
	panic("should not be called")
}

func initTestCommand(c *gc.C, store jujuclient.ClientStore, args ...string) (*testCommand, error) {
	cmd := new(testCommand)
	cmd.SetClientStore(store)
	wrapped := modelcmd.Wrap(cmd)
	return cmd, cmdtesting.InitCommand(wrapped, args)
}

type closer struct{}

func (*closer) Close() error {
	return nil
}

var _ = gc.Suite(&macaroonLoginSuite{})

type macaroonLoginSuite struct {
	apitesting.MacaroonSuite
	store          *jujuclienttesting.MemStore
	controllerName string
	modelName      string
}

const testUser = "testuser@somewhere"

func (s *macaroonLoginSuite) SetUpTest(c *gc.C) {
	s.MacaroonSuite.SetUpTest(c)
	s.MacaroonSuite.AddModelUser(c, testUser)

	s.controllerName = "my-controller"
	s.modelName = testUser + "/my-model"
	modelTag := names.NewModelTag(s.State.ModelUUID())
	apiInfo := s.APIInfo(c)

	s.store = jujuclienttesting.NewMemStore()
	s.store.Controllers[s.controllerName] = jujuclient.ControllerDetails{
		APIEndpoints:   apiInfo.Addrs,
		ControllerUUID: s.State.ControllerUUID(),
		CACert:         apiInfo.CACert,
	}
	s.store.Accounts[s.controllerName] = jujuclient.AccountDetails{
		// External user forces use of macaroons.
		User: "me@external",
	}
	s.store.Models[s.controllerName] = &jujuclient.ControllerModels{
		Models: map[string]jujuclient.ModelDetails{
			s.modelName: {modelTag.Id()},
		},
	}
}

func (s *macaroonLoginSuite) TestsSuccessfulLogin(c *gc.C) {
	s.DischargerLogin = func() string {
		return testUser
	}

	cmd := modelcmd.NewModelCommandBase(s.store, s.controllerName, s.modelName)
	_, err := cmd.NewAPIRoot()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *macaroonLoginSuite) TestsFailToObtainDischargeLogin(c *gc.C) {
	s.DischargerLogin = func() string {
		return ""
	}

	cmd := modelcmd.NewModelCommandBase(s.store, s.controllerName, s.modelName)
	_, err := cmd.NewAPIRoot()
	c.Assert(err, gc.ErrorMatches, "cannot get discharge.*")
}

func (s *macaroonLoginSuite) TestsUnknownUserLogin(c *gc.C) {
	s.DischargerLogin = func() string {
		return "testUnknown@nowhere"
	}

	cmd := modelcmd.NewModelCommandBase(s.store, s.controllerName, s.modelName)
	_, err := cmd.NewAPIRoot()
	c.Assert(err, gc.ErrorMatches, "invalid entity name or password \\(unauthorized access\\)")
}
