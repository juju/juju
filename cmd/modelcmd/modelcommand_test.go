// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcmd_test

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	apitesting "github.com/juju/juju/api/testing"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/permission"
)

type ModelCommandSuite struct {
	testing.IsolationSuite
	store *jujuclient.MemStore
}

func (s *ModelCommandSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.PatchEnvironment("JUJU_CLI_VERSION", "")

	s.store = jujuclient.NewMemStore()
	s.store.CurrentControllerName = "foo"
	s.store.Controllers["foo"] = jujuclient.ControllerDetails{}
	s.store.Accounts["foo"] = jujuclient.AccountDetails{
		User: "bar", Password: "hunter2",
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
	err := s.store.UpdateModel("foo", "admin/mymodel", jujuclient.ModelDetails{"uuid"})
	c.Assert(err, jc.ErrorIsNil)
	err = s.store.SetCurrentModel("foo", "admin/mymodel")
	c.Assert(err, jc.ErrorIsNil)

	env, err := modelcmd.GetCurrentModel(s.store)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env, gc.Equals, "foo:admin/mymodel")
}

func (s *ModelCommandSuite) TestGetCurrentModelJujuEnvSet(c *gc.C) {
	os.Setenv(osenv.JujuModelEnvKey, "admin/magic")
	env, err := modelcmd.GetCurrentModel(s.store)
	c.Assert(env, gc.Equals, "admin/magic")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ModelCommandSuite) TestGetCurrentModelBothSet(c *gc.C) {
	os.Setenv(osenv.JujuModelEnvKey, "admin/magic")

	err := s.store.UpdateModel("foo", "admin/mymodel", jujuclient.ModelDetails{"uuid"})
	c.Assert(err, jc.ErrorIsNil)
	err = s.store.SetCurrentModel("foo", "admin/mymodel")
	c.Assert(err, jc.ErrorIsNil)

	env, err := modelcmd.GetCurrentModel(s.store)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env, gc.Equals, "admin/magic")
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
	err := s.store.UpdateModel("foo", "admin/mymodel", jujuclient.ModelDetails{"uuid"})
	c.Assert(err, jc.ErrorIsNil)
	err = s.store.SetCurrentModel("foo", "admin/mymodel")
	c.Assert(err, jc.ErrorIsNil)
	s.testEnsureModelName(c, "admin/mymodel")
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

func (s *ModelCommandSuite) TestInnerCommand(c *gc.C) {
	cmd := new(testCommand)
	wrapped := modelcmd.Wrap(cmd)
	c.Assert(modelcmd.InnerCommand(wrapped), gc.Equals, cmd)
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
	cmd, err := runTestCommand(c, s.store, args...)
	c.Assert(err, jc.ErrorIsNil)
	modelName, err := cmd.ModelName()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelName, gc.Equals, expect)
}

func runTestCommand(c *gc.C, store jujuclient.ClientStore, args ...string) (modelcmd.ModelCommand, error) {
	cmd := modelcmd.Wrap(new(testCommand))
	cmd.SetClientStore(store)
	_, err := cmdtesting.RunCommand(c, cmd, args...)
	return cmd, errors.Trace(err)
}

type testCommand struct {
	modelcmd.ModelCommandBase
}

func (c *testCommand) Info() *cmd.Info {
	panic("should not be called")
}

func (c *testCommand) Run(ctx *cmd.Context) error {
	return nil
}

type closer struct{}

func (*closer) Close() error {
	return nil
}

var _ = gc.Suite(&macaroonLoginSuite{})

type macaroonLoginSuite struct {
	apitesting.MacaroonSuite
	store          *jujuclient.MemStore
	controllerName string
	modelName      string
}

const testUser = "testuser@somewhere"

func (s *macaroonLoginSuite) SetUpTest(c *gc.C) {
	s.MacaroonSuite.SetUpTest(c)
	s.MacaroonSuite.AddModelUser(c, testUser)
	s.MacaroonSuite.AddControllerUser(c, testUser, permission.LoginAccess)

	s.controllerName = "my-controller"
	s.modelName = testUser + "/my-model"
	modelTag := names.NewModelTag(s.State.ModelUUID())
	apiInfo := s.APIInfo(c)

	s.store = jujuclient.NewMemStore()
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

func (s *macaroonLoginSuite) newModelCommandBase() *modelcmd.ModelCommandBase {
	var c modelcmd.ModelCommandBase
	c.SetClientStore(s.store)
	modelcmd.InitContexts(&cmd.Context{Stderr: ioutil.Discard}, &c)
	modelcmd.SetRunStarted(&c)
	err := c.SetModelName(s.controllerName+":"+s.modelName, false)
	if err != nil {
		panic(err)
	}
	return &c
}

func (s *macaroonLoginSuite) TestsSuccessfulLogin(c *gc.C) {
	s.DischargerLogin = func() string {
		return testUser
	}

	cmd := s.newModelCommandBase()
	_, err := cmd.NewAPIRoot()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *macaroonLoginSuite) TestsFailToObtainDischargeLogin(c *gc.C) {
	s.DischargerLogin = func() string {
		return ""
	}

	cmd := s.newModelCommandBase()
	_, err := cmd.NewAPIRoot()
	c.Assert(err, gc.ErrorMatches, "cannot get discharge.*", gc.Commentf("%s", errors.Details(err)))
}

func (s *macaroonLoginSuite) TestsUnknownUserLogin(c *gc.C) {
	s.DischargerLogin = func() string {
		return "testUnknown@nowhere"
	}

	cmd := s.newModelCommandBase()
	_, err := cmd.NewAPIRoot()
	c.Assert(err, gc.ErrorMatches, "invalid entity name or password \\(unauthorized access\\)", gc.Commentf("details: %s", errors.Details(err)))
}
