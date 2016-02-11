// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcmd_test

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	goyaml "gopkg.in/yaml.v1"

	apitesting "github.com/juju/juju/api/testing"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/testing"
)

type ModelCommandSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

func (s *ModelCommandSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.PatchEnvironment("JUJU_CLI_VERSION", "")
}

var _ = gc.Suite(&ModelCommandSuite{})

func (s *ModelCommandSuite) TestGetDefaultModelNothingSet(c *gc.C) {
	env, err := modelcmd.GetDefaultModel()
	c.Assert(env, gc.Equals, "")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ModelCommandSuite) TestGetDefaultModelCurrentModelSet(c *gc.C) {
	err := modelcmd.WriteCurrentModel("fubar")
	c.Assert(err, jc.ErrorIsNil)
	env, err := modelcmd.GetDefaultModel()
	c.Assert(env, gc.Equals, "fubar")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ModelCommandSuite) TestGetDefaultModelJujuEnvSet(c *gc.C) {
	os.Setenv(osenv.JujuModelEnvKey, "magic")
	env, err := modelcmd.GetDefaultModel()
	c.Assert(env, gc.Equals, "magic")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ModelCommandSuite) TestGetDefaultModelBothSet(c *gc.C) {
	os.Setenv(osenv.JujuModelEnvKey, "magic")
	err := modelcmd.WriteCurrentModel("fubar")
	c.Assert(err, jc.ErrorIsNil)
	env, err := modelcmd.GetDefaultModel()
	c.Assert(env, gc.Equals, "magic")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ModelCommandSuite) TestModelCommandInitExplicit(c *gc.C) {
	// Take model name from command line arg.
	testEnsureModelName(c, "explicit", "-m", "explicit")
}

func (s *ModelCommandSuite) TestModelCommandInitExplicitLongForm(c *gc.C) {
	// Take model name from command line arg.
	testEnsureModelName(c, "explicit", "--model", "explicit")
}

func (s *ModelCommandSuite) TestModelCommandInitEnvFile(c *gc.C) {
	// If there is a current-model file, use that.
	err := modelcmd.WriteCurrentModel("fubar")
	c.Assert(err, jc.ErrorIsNil)
	testEnsureModelName(c, "fubar")
}

func (s *ModelCommandSuite) TestModelCommandInitControllerFile(c *gc.C) {
	// If there is a current-controller file, error raised.
	err := modelcmd.WriteCurrentController("fubar")
	c.Assert(err, jc.ErrorIsNil)
	_, err = initTestCommand(c)
	c.Assert(err, gc.ErrorMatches, `not operating on an model, using controller "fubar"`)
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
	wrapped := modelcmd.Wrap(cmd, modelcmd.ModelSkipFlags)
	args := []string{"-m", "testenv"}
	err := cmdtesting.InitCommand(wrapped, args)
	// 1st position is always the flag
	msg := fmt.Sprintf("flag provided but not defined: %v", args[0])
	c.Assert(err, gc.ErrorMatches, msg)
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

func initTestCommand(c *gc.C, args ...string) (*testCommand, error) {
	cmd := new(testCommand)
	wrapped := modelcmd.Wrap(cmd)
	return cmd, cmdtesting.InitCommand(wrapped, args)
}

func testEnsureModelName(c *gc.C, expect string, args ...string) {
	cmd, err := initTestCommand(c, args...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmd.ConnectionName(), gc.Equals, expect)
}

type ConnectionEndpointSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	store    configstore.Storage
	endpoint configstore.APIEndpoint
}

var _ = gc.Suite(&ConnectionEndpointSuite{})

func (s *ConnectionEndpointSuite) SetUpTest(c *gc.C) {
	s.FakeHomeSuite.SetUpTest(c)
	s.store = configstore.NewMem()
	s.PatchValue(modelcmd.GetConfigStore, func() (configstore.Storage, error) {
		return s.store, nil
	})
	newInfo := s.store.CreateInfo("model-name")
	newInfo.SetAPICredentials(configstore.APICredentials{
		User:     "foo",
		Password: "foopass",
	})
	s.endpoint = configstore.APIEndpoint{
		Addresses: []string{"0.1.2.3"},
		Hostnames: []string{"foo.invalid"},
		CACert:    "certificated",
		ModelUUID: "fake-uuid",
	}
	newInfo.SetAPIEndpoint(s.endpoint)
	err := newInfo.Write()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ConnectionEndpointSuite) TestAPIEndpointInStoreCached(c *gc.C) {
	cmd, err := initTestCommand(c, "-m", "model-name")
	c.Assert(err, jc.ErrorIsNil)
	endpoint, err := cmd.ConnectionEndpoint(false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(endpoint, gc.DeepEquals, s.endpoint)
}

func (s *ConnectionEndpointSuite) TestAPIEndpointForEnvSuchName(c *gc.C) {
	cmd, err := initTestCommand(c, "-m", "no-such-model")
	c.Assert(err, jc.ErrorIsNil)
	_, err = cmd.ConnectionEndpoint(false)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(err, gc.ErrorMatches, `model "no-such-model" not found`)
}

func (s *ConnectionEndpointSuite) TestAPIEndpointRefresh(c *gc.C) {
	newEndpoint := configstore.APIEndpoint{
		Addresses: []string{"0.1.2.3"},
		Hostnames: []string{"foo.example.com"},
		CACert:    "certificated",
		ModelUUID: "fake-uuid",
	}
	s.PatchValue(modelcmd.EndpointRefresher, func(_ *modelcmd.ModelCommandBase) (io.Closer, error) {
		info, err := s.store.ReadInfo("model-name")
		info.SetAPIEndpoint(newEndpoint)
		err = info.Write()
		c.Assert(err, jc.ErrorIsNil)
		return new(closer), nil
	})

	cmd, err := initTestCommand(c, "-m", "model-name")
	c.Assert(err, jc.ErrorIsNil)
	endpoint, err := cmd.ConnectionEndpoint(true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(endpoint, gc.DeepEquals, newEndpoint)
}

type closer struct{}

func (*closer) Close() error {
	return nil
}

var _ = gc.Suite(&macaroonLoginSuite{})

type macaroonLoginSuite struct {
	apitesting.MacaroonSuite
	serverFilePath string
	envName        string
}

const testUser = "testuser@somewhere"

func (s *macaroonLoginSuite) SetUpTest(c *gc.C) {
	s.MacaroonSuite.SetUpTest(c)

	modelTag := names.NewModelTag(s.State.ModelUUID())
	s.envName = modelTag.Id()

	s.MacaroonSuite.AddModelUser(c, testUser)

	apiInfo := s.APIInfo(c)
	var serverDetails modelcmd.ServerFile
	serverDetails.Addresses = apiInfo.Addrs
	serverDetails.CACert = apiInfo.CACert
	content, err := goyaml.Marshal(serverDetails)
	c.Assert(err, jc.ErrorIsNil)

	s.serverFilePath = filepath.Join(c.MkDir(), "server.yaml")

	err = ioutil.WriteFile(s.serverFilePath, content, 0644)
	c.Assert(err, jc.ErrorIsNil)

	store, err := configstore.Default()
	c.Assert(err, jc.ErrorIsNil)
	cfg := store.CreateInfo(s.envName)
	cfg.SetAPIEndpoint(configstore.APIEndpoint{
		Addresses: apiInfo.Addrs,
		Hostnames: []string{"0.1.2.3"},
		CACert:    apiInfo.CACert,
		ModelUUID: s.envName,
	})
	err = cfg.Write()
	cfg.SetAPICredentials(configstore.APICredentials{
		User:     "",
		Password: "",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *macaroonLoginSuite) TestsSuccessfulLogin(c *gc.C) {
	s.DischargerLogin = func() string {
		return testUser
	}

	cmd := modelcmd.NewModelCommandBase(s.envName, nil, nil)
	_, err := cmd.NewAPIRoot()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *macaroonLoginSuite) TestsFailToObtainDischargeLogin(c *gc.C) {
	s.DischargerLogin = func() string {
		return ""
	}

	cmd := modelcmd.NewModelCommandBase(s.envName, nil, nil)
	_, err := cmd.NewAPIRoot()
	// TODO(rog) is this really the right error here?
	c.Assert(err, gc.ErrorMatches, `model "`+s.envName+`" not found`)
}

func (s *macaroonLoginSuite) TestsUnknownUserLogin(c *gc.C) {
	s.DischargerLogin = func() string {
		return "testUnknown@nowhere"
	}

	cmd := modelcmd.NewModelCommandBase(s.envName, nil, nil)
	_, err := cmd.NewAPIRoot()
	// TODO(rog) is this really the right error here?
	c.Assert(err, gc.ErrorMatches, `model "`+s.envName+`" not found`)
}
