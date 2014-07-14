// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package envcmd_test

import (
	"io"
	"io/ioutil"
	"os"
	"testing"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/juju/osenv"
	coretesting "github.com/juju/juju/testing"
)

type EnvironmentCommandSuite struct {
	coretesting.FakeJujuHomeSuite
}

var _ = gc.Suite(&EnvironmentCommandSuite{})

func Test(t *testing.T) { gc.TestingT(t) }

func (s *EnvironmentCommandSuite) TestReadCurrentEnvironmentUnset(c *gc.C) {
	env := envcmd.ReadCurrentEnvironment()
	c.Assert(env, gc.Equals, "")
}

func (s *EnvironmentCommandSuite) TestReadCurrentEnvironmentSet(c *gc.C) {
	err := envcmd.WriteCurrentEnvironment("fubar")
	c.Assert(err, gc.IsNil)
	env := envcmd.ReadCurrentEnvironment()
	c.Assert(env, gc.Equals, "fubar")
}

func (s *EnvironmentCommandSuite) TestGetDefaultEnvironmentNothingSet(c *gc.C) {
	envPath := gitjujutesting.HomePath(".juju", "environments.yaml")
	err := os.Remove(envPath)
	c.Assert(err, gc.IsNil)
	env, err := envcmd.GetDefaultEnvironment()
	c.Assert(env, gc.Equals, "")
	c.Assert(err, jc.Satisfies, environs.IsNoEnv)
}

func (s *EnvironmentCommandSuite) TestGetDefaultEnvironmentCurrentEnvironmentSet(c *gc.C) {
	err := envcmd.WriteCurrentEnvironment("fubar")
	c.Assert(err, gc.IsNil)
	env, err := envcmd.GetDefaultEnvironment()
	c.Assert(env, gc.Equals, "fubar")
	c.Assert(err, gc.IsNil)
}

func (s *EnvironmentCommandSuite) TestGetDefaultEnvironmentJujuEnvSet(c *gc.C) {
	os.Setenv(osenv.JujuEnvEnvKey, "magic")
	env, err := envcmd.GetDefaultEnvironment()
	c.Assert(env, gc.Equals, "magic")
	c.Assert(err, gc.IsNil)
}

func (s *EnvironmentCommandSuite) TestGetDefaultEnvironmentBothSet(c *gc.C) {
	os.Setenv(osenv.JujuEnvEnvKey, "magic")
	err := envcmd.WriteCurrentEnvironment("fubar")
	c.Assert(err, gc.IsNil)
	env, err := envcmd.GetDefaultEnvironment()
	c.Assert(env, gc.Equals, "magic")
	c.Assert(err, gc.IsNil)
}

func (s *EnvironmentCommandSuite) TestWriteAddsNewline(c *gc.C) {
	err := envcmd.WriteCurrentEnvironment("fubar")
	c.Assert(err, gc.IsNil)
	current, err := ioutil.ReadFile(envcmd.GetCurrentEnvironmentFilePath())
	c.Assert(err, gc.IsNil)
	c.Assert(string(current), gc.Equals, "fubar\n")
}

func (*EnvironmentCommandSuite) TestErrorWritingFile(c *gc.C) {
	// Can't write a file over a directory.
	os.MkdirAll(envcmd.GetCurrentEnvironmentFilePath(), 0777)
	err := envcmd.WriteCurrentEnvironment("fubar")
	c.Assert(err, gc.ErrorMatches, "unable to write to the environment file: .*")
}

func (s *EnvironmentCommandSuite) TestEnvironCommandInit(c *gc.C) {
	// Take environment name from command line arg.
	testEnsureEnvName(c, "explicit", "-e", "explicit")

	// Take environment name from the default.
	coretesting.WriteEnvironments(c, coretesting.MultipleEnvConfig)
	testEnsureEnvName(c, coretesting.SampleEnvName)

	// Take environment name from the one and only environment,
	// even if it is not explicitly marked as default.
	coretesting.WriteEnvironments(c, coretesting.SingleEnvConfigNoDefault)
	testEnsureEnvName(c, coretesting.SampleEnvName)

	// If there is a current-environment file, use that.
	err := envcmd.WriteCurrentEnvironment("fubar")
	c.Assert(err, gc.IsNil)
	testEnsureEnvName(c, "fubar")
}

func (s *EnvironmentCommandSuite) TestEnvironCommandInitErrors(c *gc.C) {
	envPath := gitjujutesting.HomePath(".juju", "environments.yaml")
	err := os.Remove(envPath)
	c.Assert(err, gc.IsNil)
	cmd := envcmd.Wrap(new(testCommand))
	err = cmdtesting.InitCommand(cmd, nil)
	c.Assert(err, jc.Satisfies, environs.IsNoEnv)

	// If there are multiple environments but no default,
	// an error should be returned.
	coretesting.WriteEnvironments(c, coretesting.MultipleEnvConfigNoDefault)
	err = cmdtesting.InitCommand(cmd, nil)
	c.Assert(err, gc.Equals, envcmd.ErrNoEnvironmentSpecified)
}

type testCommand struct {
	envcmd.EnvCommandBase
}

func (c *testCommand) Info() *cmd.Info {
	panic("should not be called")
}

func (c *testCommand) Run(ctx *cmd.Context) error {
	panic("should not be called")
}

func initTestCommand(c *gc.C, args ...string) (*testCommand, error) {
	cmd := new(testCommand)
	wrapped := envcmd.Wrap(cmd)
	return cmd, cmdtesting.InitCommand(wrapped, args)
}

func testEnsureEnvName(c *gc.C, expect string, args ...string) {
	cmd, err := initTestCommand(c, args...)
	c.Assert(err, gc.IsNil)
	c.Assert(cmd.ConnectionName(), gc.Equals, expect)
}

type ConnectionEndpointSuite struct {
	coretesting.FakeJujuHomeSuite
	store    configstore.Storage
	endpoint configstore.APIEndpoint
}

var _ = gc.Suite(&ConnectionEndpointSuite{})

func (s *ConnectionEndpointSuite) SetUpTest(c *gc.C) {
	s.FakeHomeSuite.SetUpTest(c)
	s.store = configstore.NewMem()
	s.PatchValue(envcmd.GetConfigStore, func() (configstore.Storage, error) {
		return s.store, nil
	})
	newInfo, err := s.store.CreateInfo("env-name")
	c.Assert(err, gc.IsNil)
	newInfo.SetAPICredentials(configstore.APICredentials{
		User:     "foo",
		Password: "foopass",
	})
	s.endpoint = configstore.APIEndpoint{
		Addresses:   []string{"foo.invalid"},
		CACert:      "certificated",
		EnvironUUID: "fake-uuid",
	}
	newInfo.SetAPIEndpoint(s.endpoint)
	err = newInfo.Write()
	c.Assert(err, gc.IsNil)
}

func (s *ConnectionEndpointSuite) TestAPIEndpointInStoreCached(c *gc.C) {
	cmd, err := initTestCommand(c, "-e", "env-name")
	c.Assert(err, gc.IsNil)
	endpoint, err := cmd.ConnectionEndpoint(false)
	c.Assert(err, gc.IsNil)
	c.Assert(endpoint, gc.DeepEquals, s.endpoint)
}

func (s *ConnectionEndpointSuite) TestAPIEndpointForEnvSuchName(c *gc.C) {
	cmd, err := initTestCommand(c, "-e", "no-such-env")
	c.Assert(err, gc.IsNil)
	_, err = cmd.ConnectionEndpoint(false)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(err, gc.ErrorMatches, `environment "no-such-env" not found`)
}

func (s *ConnectionEndpointSuite) TestAPIEndpointRefresh(c *gc.C) {
	newEndpoint := configstore.APIEndpoint{
		Addresses:   []string{"foo.example.com"},
		CACert:      "certificated",
		EnvironUUID: "fake-uuid",
	}
	s.PatchValue(envcmd.EndpointRefresher, func(_ *envcmd.EnvCommandBase) (io.Closer, error) {
		info, err := s.store.ReadInfo("env-name")
		info.SetAPIEndpoint(newEndpoint)
		err = info.Write()
		c.Assert(err, gc.IsNil)
		return new(closer), nil
	})

	cmd, err := initTestCommand(c, "-e", "env-name")
	c.Assert(err, gc.IsNil)
	endpoint, err := cmd.ConnectionEndpoint(true)
	c.Assert(err, gc.IsNil)
	c.Assert(endpoint, gc.DeepEquals, newEndpoint)
}

type closer struct{}

func (*closer) Close() error {
	return nil
}
