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
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/juju/osenv"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type EnvironmentCommandSuite struct {
	coretesting.FakeJujuHomeSuite
}

func (s *EnvironmentCommandSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)
	s.PatchEnvironment("JUJU_CLI_VERSION", "")
}

var _ = gc.Suite(&EnvironmentCommandSuite{})

func Test(t *testing.T) { gc.TestingT(t) }

func (s *EnvironmentCommandSuite) TestReadCurrentEnvironmentUnset(c *gc.C) {
	env := envcmd.ReadCurrentEnvironment()
	c.Assert(env, gc.Equals, "")
}

func (s *EnvironmentCommandSuite) TestReadCurrentEnvironmentSet(c *gc.C) {
	err := envcmd.WriteCurrentEnvironment("fubar")
	c.Assert(err, jc.ErrorIsNil)
	env := envcmd.ReadCurrentEnvironment()
	c.Assert(env, gc.Equals, "fubar")
}

func (s *EnvironmentCommandSuite) TestGetDefaultEnvironment(c *gc.C) {
	env, err := envcmd.GetDefaultEnvironment()
	c.Assert(env, gc.Equals, "erewhemos")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *EnvironmentCommandSuite) TestGetDefaultEnvironmentNothingSet(c *gc.C) {
	envPath := gitjujutesting.HomePath(".juju", "environments.yaml")
	err := os.Remove(envPath)
	c.Assert(err, jc.ErrorIsNil)
	env, err := envcmd.GetDefaultEnvironment()
	c.Assert(env, gc.Equals, "")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *EnvironmentCommandSuite) TestGetDefaultEnvironmentCurrentEnvironmentSet(c *gc.C) {
	err := envcmd.WriteCurrentEnvironment("fubar")
	c.Assert(err, jc.ErrorIsNil)
	env, err := envcmd.GetDefaultEnvironment()
	c.Assert(env, gc.Equals, "fubar")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *EnvironmentCommandSuite) TestGetDefaultEnvironmentJujuEnvSet(c *gc.C) {
	os.Setenv(osenv.JujuEnvEnvKey, "magic")
	env, err := envcmd.GetDefaultEnvironment()
	c.Assert(env, gc.Equals, "magic")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *EnvironmentCommandSuite) TestGetDefaultEnvironmentBothSet(c *gc.C) {
	os.Setenv(osenv.JujuEnvEnvKey, "magic")
	err := envcmd.WriteCurrentEnvironment("fubar")
	c.Assert(err, jc.ErrorIsNil)
	env, err := envcmd.GetDefaultEnvironment()
	c.Assert(env, gc.Equals, "magic")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *EnvironmentCommandSuite) TestWriteAddsNewline(c *gc.C) {
	err := envcmd.WriteCurrentEnvironment("fubar")
	c.Assert(err, jc.ErrorIsNil)
	current, err := ioutil.ReadFile(envcmd.GetCurrentEnvironmentFilePath())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(current), gc.Equals, "fubar\n")
}

func (*EnvironmentCommandSuite) TestErrorWritingFile(c *gc.C) {
	// Can't write a file over a directory.
	os.MkdirAll(envcmd.GetCurrentEnvironmentFilePath(), 0777)
	err := envcmd.WriteCurrentEnvironment("fubar")
	c.Assert(err, gc.ErrorMatches, "unable to write to the environment file: .*")
}

func (s *EnvironmentCommandSuite) TestEnvironCommandInitExplicit(c *gc.C) {
	// Take environment name from command line arg.
	testEnsureEnvName(c, "explicit", "-e", "explicit")
}

func (s *EnvironmentCommandSuite) TestEnvironCommandInitMultipleConfigs(c *gc.C) {
	// Take environment name from the default.
	coretesting.WriteEnvironments(c, coretesting.MultipleEnvConfig)
	testEnsureEnvName(c, coretesting.SampleEnvName)
}

func (s *EnvironmentCommandSuite) TestEnvironCommandInitSingleConfig(c *gc.C) {
	// Take environment name from the one and only environment,
	// even if it is not explicitly marked as default.
	coretesting.WriteEnvironments(c, coretesting.SingleEnvConfigNoDefault)
	testEnsureEnvName(c, coretesting.SampleEnvName)
}

func (s *EnvironmentCommandSuite) TestEnvironCommandInitEnvFile(c *gc.C) {
	// If there is a current-environment file, use that.
	err := envcmd.WriteCurrentEnvironment("fubar")
	c.Assert(err, jc.ErrorIsNil)
	testEnsureEnvName(c, "fubar")
}

func (s *EnvironmentCommandSuite) TestEnvironCommandInitNoEnvFile(c *gc.C) {
	envPath := gitjujutesting.HomePath(".juju", "environments.yaml")
	err := os.Remove(envPath)
	c.Assert(err, jc.ErrorIsNil)
	testEnsureEnvName(c, "")
}

func (s *EnvironmentCommandSuite) TestEnvironCommandInitMultipleConfigNoDefault(c *gc.C) {
	// If there are multiple environments but no default, the connection name is empty.
	coretesting.WriteEnvironments(c, coretesting.MultipleEnvConfigNoDefault)
	testEnsureEnvName(c, "")
}

func (s *EnvironmentCommandSuite) TestBootstrapContext(c *gc.C) {
	ctx := envcmd.BootstrapContext(&cmd.Context{})
	c.Assert(ctx.ShouldVerifyCredentials(), jc.IsTrue)
}

func (s *EnvironmentCommandSuite) TestBootstrapContextNoVerify(c *gc.C) {
	ctx := envcmd.BootstrapContextNoVerify(&cmd.Context{})
	c.Assert(ctx.ShouldVerifyCredentials(), jc.IsFalse)
}

func (s *EnvironmentCommandSuite) TestCompatVersion(c *gc.C) {
	s.PatchEnvironment(osenv.JujuCLIVersion, "2")
	cmd, err := initTestCommand(c)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmd.CompatVersion(), gc.Equals, 2)
}

func (s *EnvironmentCommandSuite) TestCompatVersionDefault(c *gc.C) {
	cmd, err := initTestCommand(c)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmd.CompatVersion(), gc.Equals, 1)
}

func (s *EnvironmentCommandSuite) TestCompatVersionInvalid(c *gc.C) {
	s.PatchEnvironment(osenv.JujuCLIVersion, "invalid")
	cmd, err := initTestCommand(c)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmd.CompatVersion(), gc.Equals, 1)
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
	c.Assert(err, jc.ErrorIsNil)
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
	newInfo := s.store.CreateInfo("env-name")
	newInfo.SetAPICredentials(configstore.APICredentials{
		User:     "foo",
		Password: "foopass",
	})
	s.endpoint = configstore.APIEndpoint{
		Addresses:   []string{"0.1.2.3"},
		Hostnames:   []string{"foo.invalid"},
		CACert:      "certificated",
		EnvironUUID: "fake-uuid",
	}
	newInfo.SetAPIEndpoint(s.endpoint)
	err := newInfo.Write()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ConnectionEndpointSuite) TestAPIEndpointInStoreCached(c *gc.C) {
	cmd, err := initTestCommand(c, "-e", "env-name")
	c.Assert(err, jc.ErrorIsNil)
	endpoint, err := cmd.ConnectionEndpoint(false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(endpoint, gc.DeepEquals, s.endpoint)
}

func (s *ConnectionEndpointSuite) TestAPIEndpointForEnvSuchName(c *gc.C) {
	cmd, err := initTestCommand(c, "-e", "no-such-env")
	c.Assert(err, jc.ErrorIsNil)
	_, err = cmd.ConnectionEndpoint(false)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(err, gc.ErrorMatches, `environment "no-such-env" not found`)
}

func (s *ConnectionEndpointSuite) TestAPIEndpointRefresh(c *gc.C) {
	newEndpoint := configstore.APIEndpoint{
		Addresses:   []string{"0.1.2.3"},
		Hostnames:   []string{"foo.example.com"},
		CACert:      "certificated",
		EnvironUUID: "fake-uuid",
	}
	s.PatchValue(envcmd.EndpointRefresher, func(_ *envcmd.EnvCommandBase) (io.Closer, error) {
		info, err := s.store.ReadInfo("env-name")
		info.SetAPIEndpoint(newEndpoint)
		err = info.Write()
		c.Assert(err, jc.ErrorIsNil)
		return new(closer), nil
	})

	cmd, err := initTestCommand(c, "-e", "env-name")
	c.Assert(err, jc.ErrorIsNil)
	endpoint, err := cmd.ConnectionEndpoint(true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(endpoint, gc.DeepEquals, newEndpoint)
}

type closer struct{}

func (*closer) Close() error {
	return nil
}

type EnvironmentVersionSuite struct {
	fake *fakeEnvGetter
}

var _ = gc.Suite(&EnvironmentVersionSuite{})

type fakeEnvGetter struct {
	agentVersion interface{}
	err          error
}

func (g *fakeEnvGetter) EnvironmentGet() (map[string]interface{}, error) {
	if g.err != nil {
		return nil, g.err
	} else if g.agentVersion == nil {
		return map[string]interface{}{}, nil
	} else {
		return map[string]interface{}{
			"agent-version": g.agentVersion,
		}, nil
	}
}

func (s *EnvironmentVersionSuite) SetUpTest(*gc.C) {
	s.fake = new(fakeEnvGetter)
}

func (s *EnvironmentVersionSuite) TestApiCallFails(c *gc.C) {
	s.fake.err = errors.New("boom")
	_, err := envcmd.GetEnvironmentVersion(s.fake)
	c.Assert(err, gc.ErrorMatches, "unable to retrieve environment config: boom")
}

func (s *EnvironmentVersionSuite) TestNoVersion(c *gc.C) {
	_, err := envcmd.GetEnvironmentVersion(s.fake)
	c.Assert(err, gc.ErrorMatches, "version not found in environment config")
}

func (s *EnvironmentVersionSuite) TestInvalidVersionType(c *gc.C) {
	s.fake.agentVersion = 99
	_, err := envcmd.GetEnvironmentVersion(s.fake)
	c.Assert(err, gc.ErrorMatches, "invalid environment version type in config")
}

func (s *EnvironmentVersionSuite) TestInvalidVersion(c *gc.C) {
	s.fake.agentVersion = "a.b.c"
	_, err := envcmd.GetEnvironmentVersion(s.fake)
	c.Assert(err, gc.ErrorMatches, "unable to parse environment version: .+")
}

func (s *EnvironmentVersionSuite) TestSuccess(c *gc.C) {
	vs := "1.22.1"
	s.fake.agentVersion = vs
	v, err := envcmd.GetEnvironmentVersion(s.fake)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(v.Compare(version.MustParse(vs)), gc.Equals, 0)
}
