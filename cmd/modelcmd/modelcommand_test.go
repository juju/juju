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
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api"
	apitesting "github.com/juju/juju/api/testing"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient"
)

type ModelCommandSuite struct {
	testing.IsolationSuite
	store *jujuclient.MemStore
}

func (s *ModelCommandSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.PatchEnvironment("JUJU_CLI_VERSION", "")

	s.store = jujuclient.NewMemStore()
}

var _ = gc.Suite(&ModelCommandSuite{})

var modelCommandModelTests = []struct {
	about            string
	args             []string
	modelEnvVar      string
	expectController string
	expectModel      string
}{{
	about:            "explicit controller and model, long form",
	args:             []string{"--model", "bar:noncurrentbar"},
	expectController: "bar",
	expectModel:      "noncurrentbar",
}, {
	about:            "explicit controller and model, short form",
	args:             []string{"-m", "bar:noncurrentbar"},
	expectController: "bar",
	expectModel:      "noncurrentbar",
}, {
	about:            "explicit controller and model UUID, long form",
	args:             []string{"--model", "bar:uuidbar2"},
	expectController: "bar",
	expectModel:      "uuidbar2",
}, {
	about:            "explicit controller and model UUID, short form",
	args:             []string{"-m", "bar:uuidbar2"},
	expectController: "bar",
	expectModel:      "uuidbar2",
}, {
	about:            "implicit controller, explicit model, short form",
	args:             []string{"-m", "explicit"},
	expectController: "foo",
	expectModel:      "explicit",
}, {
	about:            "implicit controller, explicit model, long form",
	args:             []string{"--model", "explicit"},
	expectController: "foo",
	expectModel:      "explicit",
}, {
	about:            "implicit controller, explicit model UUID, short form",
	args:             []string{"-m", "uuidfoo3"},
	expectController: "foo",
	expectModel:      "uuidfoo3",
}, {
	about:            "implicit controller, explicit model UUID, long form",
	args:             []string{"--model", "uuidfoo3"},
	expectController: "foo",
	expectModel:      "uuidfoo3",
}, {
	about:            "explicit controller, implicit model",
	args:             []string{"--model", "bar:"},
	expectController: "bar",
	expectModel:      "adminbar/currentbar",
}, {
	about:            "controller and model in env var",
	modelEnvVar:      "bar:noncurrentbar",
	expectController: "bar",
	expectModel:      "noncurrentbar",
}, {
	about:            "model only in env var",
	modelEnvVar:      "noncurrentfoo",
	expectController: "foo",
	expectModel:      "noncurrentfoo",
}, {
	about:            "controller only in env var",
	modelEnvVar:      "bar:",
	expectController: "bar",
	expectModel:      "adminbar/currentbar",
}, {
	about:            "explicit overrides model env var",
	modelEnvVar:      "foo:noncurrentbar",
	args:             []string{"-m", "noncurrentfoo"},
	expectController: "foo",
	expectModel:      "noncurrentfoo",
}, {
	about:            "explicit overrides controller & store env var",
	modelEnvVar:      "bar:noncurrentbar",
	args:             []string{"-m", "foo:noncurrentfoo"},
	expectController: "foo",
	expectModel:      "noncurrentfoo",
}}

func (s *ModelCommandSuite) TestModelIdentifier(c *gc.C) {
	s.store.Controllers["foo"] = jujuclient.ControllerDetails{}
	s.store.Controllers["bar"] = jujuclient.ControllerDetails{}
	s.store.CurrentControllerName = "foo"
	s.store.Accounts["foo"] = jujuclient.AccountDetails{
		User: "bar", Password: "hunter2",
	}
	s.store.Accounts["bar"] = jujuclient.AccountDetails{
		User: "baz", Password: "hunter3",
	}

	err := s.store.UpdateModel("foo", "adminfoo/currentfoo",
		jujuclient.ModelDetails{ModelUUID: "uuidfoo1", ModelType: model.IAAS})
	c.Assert(err, jc.ErrorIsNil)

	err = s.store.UpdateModel("foo", "adminfoo/noncurrentfoo",
		jujuclient.ModelDetails{ModelUUID: "uuidfoo2", ModelType: model.IAAS})
	c.Assert(err, jc.ErrorIsNil)

	err = s.store.UpdateModel("foo", "bar/explicit",
		jujuclient.ModelDetails{ModelUUID: "uuidfoo3", ModelType: model.IAAS})
	c.Assert(err, jc.ErrorIsNil)

	err = s.store.UpdateModel("foo", "bar/noncurrentfoo",
		jujuclient.ModelDetails{ModelUUID: "uuidfoo4", ModelType: model.IAAS})
	c.Assert(err, jc.ErrorIsNil)

	err = s.store.UpdateModel("bar", "adminbar/currentbar",
		jujuclient.ModelDetails{ModelUUID: "uuidbar1", ModelType: model.IAAS})
	c.Assert(err, jc.ErrorIsNil)

	err = s.store.UpdateModel("bar", "adminbar/noncurrentbar",
		jujuclient.ModelDetails{ModelUUID: "uuidbar2", ModelType: model.IAAS})
	c.Assert(err, jc.ErrorIsNil)

	err = s.store.UpdateModel("bar", "baz/noncurrentbar",
		jujuclient.ModelDetails{ModelUUID: "uuidbar3", ModelType: model.IAAS})
	c.Assert(err, jc.ErrorIsNil)

	err = s.store.SetCurrentModel("foo", "adminfoo/currentfoo")
	c.Assert(err, jc.ErrorIsNil)

	err = s.store.SetCurrentModel("bar", "adminbar/currentbar")
	c.Assert(err, jc.ErrorIsNil)

	for i, test := range modelCommandModelTests {
		c.Logf("test %d: %v", i, test.about)
		os.Setenv(osenv.JujuModelEnvKey, test.modelEnvVar)
		s.assertRunHasModel(c, test.expectController, test.expectModel, test.args...)
	}
}

func (s *ModelCommandSuite) TestModelType(c *gc.C) {
	s.store.Controllers["foo"] = jujuclient.ControllerDetails{}
	s.store.CurrentControllerName = "foo"
	s.store.Accounts["foo"] = jujuclient.AccountDetails{
		User: "bar", Password: "hunter2",
	}
	err := s.store.UpdateModel("foo", "adminfoo/currentfoo",
		jujuclient.ModelDetails{ModelUUID: "uuidfoo1", ModelType: model.IAAS})
	c.Assert(err, jc.ErrorIsNil)
	err = s.store.SetCurrentModel("foo", "adminfoo/currentfoo")
	c.Assert(err, jc.ErrorIsNil)

	cmd, err := runTestCommand(c, s.store)
	c.Assert(err, jc.ErrorIsNil)
	modelType, err := cmd.ModelType()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelType, gc.Equals, model.IAAS)
}

func (s *ModelCommandSuite) TestModelGeneration(c *gc.C) {
	s.store.Controllers["foo"] = jujuclient.ControllerDetails{}
	s.store.CurrentControllerName = "foo"
	s.store.Accounts["foo"] = jujuclient.AccountDetails{
		User: "bar", Password: "hunter2",
	}
	err := s.store.UpdateModel("foo", "adminfoo/currentfoo",
		jujuclient.ModelDetails{ModelUUID: "uuidfoo1", ModelType: model.IAAS, ActiveBranch: "new-branch"})
	c.Assert(err, jc.ErrorIsNil)
	err = s.store.SetCurrentModel("foo", "adminfoo/currentfoo")
	c.Assert(err, jc.ErrorIsNil)

	cmd, err := runTestCommand(c, s.store)
	c.Assert(err, jc.ErrorIsNil)
	modelGeneration, err := cmd.ActiveBranch()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelGeneration, gc.Equals, "new-branch")

	c.Assert(cmd.SetActiveBranch(model.GenerationMaster), jc.ErrorIsNil)
	modelGeneration, err = cmd.ActiveBranch()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelGeneration, gc.Equals, model.GenerationMaster)
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
	args := []string{"-m", "testmodel"}
	err := cmdtesting.InitCommand(wrapped, args)
	// 1st position is always the flag
	msg := fmt.Sprintf("option provided but not defined: %v", args[0])
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

// assertRunHasModel asserts that a command, when run with the given arguments,
// ends up with the given controller and model names.
func (s *ModelCommandSuite) assertRunHasModel(c *gc.C, expectControllerName, expectModelName string, args ...string) {
	cmd, err := runTestCommand(c, s.store, args...)
	c.Assert(err, jc.ErrorIsNil)

	controllerName, err := cmd.ControllerName()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllerName, gc.Equals, expectControllerName)

	modelName, err := cmd.ModelIdentifier()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelName, gc.Equals, expectModelName)
}

func (s *ModelCommandSuite) TestIAASOnlyCommandIAASModel(c *gc.C) {
	s.setupIAASModel(c)

	cmd, err := runTestCommand(c, s.store)
	c.Assert(err, jc.ErrorIsNil)

	modelType, err := cmd.ModelType()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelType, gc.Equals, model.IAAS)
}

func (s *ModelCommandSuite) TestIAASOnlyCommandCAASModel(c *gc.C) {
	s.store.Controllers["foo"] = jujuclient.ControllerDetails{}
	s.store.CurrentControllerName = "foo"
	s.store.Accounts["foo"] = jujuclient.AccountDetails{
		User: "bar", Password: "hunter2",
	}
	err := s.store.UpdateModel("foo", "bar/currentfoo",
		jujuclient.ModelDetails{ModelUUID: "uuidfoo1", ModelType: model.CAAS})
	c.Assert(err, jc.ErrorIsNil)
	err = s.store.SetCurrentModel("foo", "bar/currentfoo")
	c.Assert(err, jc.ErrorIsNil)

	_, err = runTestCommand(c, s.store)
	c.Assert(err, gc.ErrorMatches, `Juju command "test-command" not supported on kubernetes models`)
}

func (s *ModelCommandSuite) TestCAASOnlyCommandIAASModel(c *gc.C) {
	s.setupIAASModel(c)

	_, err := runCaasCommand(c, s.store)
	c.Assert(err, gc.ErrorMatches, `Juju command "caas-command" not supported on non-container models`)
}

func (s *ModelCommandSuite) TestAllowedCommandCAASModel(c *gc.C) {
	s.store.Controllers["foo"] = jujuclient.ControllerDetails{}
	s.store.CurrentControllerName = "foo"
	s.store.Accounts["foo"] = jujuclient.AccountDetails{
		User: "bar", Password: "hunter2",
	}
	err := s.store.UpdateModel("foo", "bar/currentfoo",
		jujuclient.ModelDetails{ModelUUID: "uuidfoo1", ModelType: model.CAAS})
	c.Assert(err, jc.ErrorIsNil)
	err = s.store.SetCurrentModel("foo", "bar/currentfoo")
	c.Assert(err, jc.ErrorIsNil)

	cmd, err := runAllowedCAASCommand(c, s.store)
	c.Assert(err, jc.ErrorIsNil)
	modelType, err := cmd.ModelType()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelType, gc.Equals, model.CAAS)
}

func (s *ModelCommandSuite) TestPartialModelUUIDSuccess(c *gc.C) {
	s.setupIAASModel(c)

	cmd, err := runTestCommand(c, s.store, "-m", "uuidfoo")
	c.Assert(err, jc.ErrorIsNil)

	modelType, err := cmd.ModelType()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelType, gc.Equals, model.IAAS)
}

func (s *ModelCommandSuite) TestPartialModelUUIDTooShortError(c *gc.C) {
	s.setupIAASModel(c)

	_, err := runTestCommand(c, s.store, "-m", "uuidf")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ModelCommandSuite) setupIAASModel(c *gc.C) {
	s.store.Controllers["foo"] = jujuclient.ControllerDetails{}
	s.store.CurrentControllerName = "foo"
	s.store.Accounts["foo"] = jujuclient.AccountDetails{
		User: "bar", Password: "hunter2",
	}
	err := s.store.UpdateModel("foo", "bar/currentfoo",
		jujuclient.ModelDetails{ModelUUID: "uuidfoo1", ModelType: model.IAAS})
	c.Assert(err, jc.ErrorIsNil)

	err = s.store.SetCurrentModel("foo", "bar/currentfoo")
	c.Assert(err, jc.ErrorIsNil)
}

func noOpRefresh(_ jujuclient.ClientStore, _ string) error {
	return nil
}

func runTestCommand(c *gc.C, store jujuclient.ClientStore, args ...string) (modelcmd.ModelCommand, error) {
	modelCmd := new(testCommand)
	modelcmd.SetModelRefresh(noOpRefresh, modelCmd)
	cmd := modelcmd.Wrap(modelCmd)
	cmd.SetClientStore(store)
	_, err := cmdtesting.RunCommand(c, cmd, args...)
	return cmd, errors.Trace(err)
}

type testCommand struct {
	modelcmd.ModelCommandBase
	modelcmd.IAASOnlyCommand
}

func (c *testCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name: "test-command",
	})
}

func (c *testCommand) Run(ctx *cmd.Context) error {
	return nil
}

func runCaasCommand(c *gc.C, store jujuclient.ClientStore, args ...string) (modelcmd.ModelCommand, error) {
	modelCmd := new(caasCommand)
	modelcmd.SetModelRefresh(noOpRefresh, modelCmd)
	cmd := modelcmd.Wrap(modelCmd)
	cmd.SetClientStore(store)
	_, err := cmdtesting.RunCommand(c, cmd, args...)
	return cmd, errors.Trace(err)
}

type caasCommand struct {
	modelcmd.ModelCommandBase
	modelcmd.CAASOnlyCommand
}

func (c *caasCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name: "caas-command",
	})
}

func (c *caasCommand) Run(ctx *cmd.Context) error {
	return nil
}

func runAllowedCAASCommand(c *gc.C, store jujuclient.ClientStore, args ...string) (modelcmd.ModelCommand, error) {
	modelCmd := new(allowedCAASCommand)
	modelcmd.SetModelRefresh(noOpRefresh, modelCmd)
	cmd := modelcmd.Wrap(modelCmd)
	cmd.SetClientStore(store)
	_, err := cmdtesting.RunCommand(c, cmd, args...)
	return cmd, errors.Trace(err)
}

type allowedCAASCommand struct {
	modelcmd.ModelCommandBase
}

func (c *allowedCAASCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name: "allowed-caas-command",
	})
}

func (c *allowedCAASCommand) Run(ctx *cmd.Context) error {
	return nil
}

var _ = gc.Suite(&macaroonLoginSuite{})

type macaroonLoginSuite struct {
	apitesting.MacaroonSuite
	store          *jujuclient.MemStore
	controllerName string
	modelName      string
	apiOpen        api.OpenFunc
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
			s.modelName: {ModelUUID: modelTag.Id(), ModelType: model.IAAS},
		},
	}
	s.apiOpen = func(info *api.Info, dialOpts api.DialOpts) (api.Connection, error) {
		mac, err := apitesting.NewMacaroon("test")
		c.Assert(err, jc.ErrorIsNil)
		info.Macaroons = []macaroon.Slice{{mac}}
		return api.Open(info, dialOpts)
	}
}

func (s *macaroonLoginSuite) newModelCommandBase() *modelcmd.ModelCommandBase {
	var c modelcmd.ModelCommandBase
	c.SetClientStore(s.store)
	modelcmd.InitContexts(&cmd.Context{Stderr: ioutil.Discard}, &c)
	modelcmd.SetRunStarted(&c)
	err := c.SetModelIdentifier(s.controllerName+":"+s.modelName, false)
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
	cmd.SetAPIOpen(s.apiOpen)
	_, err := cmd.NewAPIRoot()
	c.Assert(err, gc.ErrorMatches, "cannot get discharge.*", gc.Commentf("%s", errors.Details(err)))
}

func (s *macaroonLoginSuite) TestsUnknownUserLogin(c *gc.C) {
	s.DischargerLogin = func() string {
		return "testUnknown@nowhere"
	}

	cmd := s.newModelCommandBase()
	cmd.SetAPIOpen(s.apiOpen)
	_, err := cmd.NewAPIRoot()
	c.Assert(err, gc.ErrorMatches, "invalid entity name or password \\(unauthorized access\\)", gc.Commentf("details: %s", errors.Details(err)))
}
