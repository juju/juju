// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package envcmd_test

import (
	"io"
	"os"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type EnvironmentCommandSuite struct {
	testing.FakeJujuHomeSuite
}

func (s *EnvironmentCommandSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)
	s.PatchEnvironment("JUJU_CLI_VERSION", "")
}

var _ = gc.Suite(&EnvironmentCommandSuite{})

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

func (s *EnvironmentCommandSuite) TestEnvironCommandInitExplicit(c *gc.C) {
	// Take environment name from command line arg.
	testEnsureEnvName(c, "explicit", "-e", "explicit")
}

func (s *EnvironmentCommandSuite) TestEnvironCommandInitMultipleConfigs(c *gc.C) {
	// Take environment name from the default.
	testing.WriteEnvironments(c, testing.MultipleEnvConfig)
	testEnsureEnvName(c, testing.SampleEnvName)
}

func (s *EnvironmentCommandSuite) TestEnvironCommandInitSingleConfig(c *gc.C) {
	// Take environment name from the one and only environment,
	// even if it is not explicitly marked as default.
	testing.WriteEnvironments(c, testing.SingleEnvConfigNoDefault)
	testEnsureEnvName(c, testing.SampleEnvName)
}

func (s *EnvironmentCommandSuite) TestEnvironCommandInitEnvFile(c *gc.C) {
	// If there is a current-environment file, use that.
	err := envcmd.WriteCurrentEnvironment("fubar")
	c.Assert(err, jc.ErrorIsNil)
	testEnsureEnvName(c, "fubar")
}

func (s *EnvironmentCommandSuite) TestEnvironCommandInitSystemFile(c *gc.C) {
	// If there is a current-system file, error raised.
	err := envcmd.WriteCurrentSystem("fubar")
	c.Assert(err, jc.ErrorIsNil)
	_, err = initTestCommand(c)
	c.Assert(err, gc.ErrorMatches, `not operating on an environment, using system "fubar"`)
}

func (s *EnvironmentCommandSuite) TestEnvironCommandInitNoEnvFile(c *gc.C) {
	envPath := gitjujutesting.HomePath(".juju", "environments.yaml")
	err := os.Remove(envPath)
	c.Assert(err, jc.ErrorIsNil)
	testEnsureEnvName(c, "")
}

func (s *EnvironmentCommandSuite) TestEnvironCommandInitMultipleConfigNoDefault(c *gc.C) {
	// If there are multiple environments but no default, the connection name is empty.
	testing.WriteEnvironments(c, testing.MultipleEnvConfigNoDefault)
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
	testing.FakeJujuHomeSuite
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
	results      map[string]interface{}
	closeCalled  bool
	getCalled    bool
}

func (g *fakeEnvGetter) EnvironmentGet() (map[string]interface{}, error) {
	g.getCalled = true
	if g.err != nil {
		return nil, g.err
	} else if g.results != nil {
		return g.results, nil
	} else if g.agentVersion == nil {
		return map[string]interface{}{}, nil
	} else {
		return map[string]interface{}{
			"agent-version": g.agentVersion,
		}, nil
	}
}

func (g *fakeEnvGetter) Close() error {
	g.closeCalled = true
	return nil
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

type EnvConfigSuite struct {
	testing.FakeJujuHomeSuite
	client  *fakeEnvGetter
	store   configstore.Storage
	envName string
}

var _ = gc.Suite(&EnvConfigSuite{})

func createBootstrapInfo(c *gc.C, name string) map[string]interface{} {
	bootstrapCfg, err := config.New(config.UseDefaults, map[string]interface{}{
		"type":         "dummy",
		"name":         name,
		"state-server": "true",
		"state-id":     "1",
	})
	c.Assert(err, jc.ErrorIsNil)
	return bootstrapCfg.AllAttrs()
}

func (s *EnvConfigSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)
	s.envName = "test-env"
	s.client = &fakeEnvGetter{results: createBootstrapInfo(c, s.envName)}

	var err error
	s.store, err = configstore.Default()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *EnvConfigSuite) writeStore(c *gc.C, bootstrapInfo bool) {
	info := s.store.CreateInfo(s.envName)
	info.SetAPIEndpoint(configstore.APIEndpoint{
		Addresses:   []string{"localhost"},
		CACert:      testing.CACert,
		EnvironUUID: s.envName + "-UUID",
		ServerUUID:  s.envName + "-UUID",
	})

	if bootstrapInfo {
		info.SetBootstrapConfig(createBootstrapInfo(c, s.envName))
	}
	err := info.Write()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *EnvConfigSuite) TestConfigWithBootstrapInfo(c *gc.C) {
	cmd := envcmd.NewEnvCommandBase(s.envName, s.client, nil)
	s.writeStore(c, true)

	cfg, err := cmd.Config(s.store, s.client)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cfg.Name(), gc.Equals, s.envName)
	c.Check(s.client.getCalled, jc.IsFalse)
	c.Check(s.client.closeCalled, jc.IsFalse)
}

func (s *EnvConfigSuite) TestConfigWithNoBootstrapWithClient(c *gc.C) {
	cmd := envcmd.NewEnvCommandBase(s.envName, s.client, nil)
	s.writeStore(c, false)

	cfg, err := cmd.Config(s.store, s.client)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cfg.Name(), gc.Equals, s.envName)
	c.Check(s.client.getCalled, jc.IsTrue)
	c.Check(s.client.closeCalled, jc.IsFalse)
}

func (s *EnvConfigSuite) TestConfigWithNoBootstrapNoClient(c *gc.C) {
	cmd := envcmd.NewEnvCommandBase(s.envName, s.client, nil)
	s.writeStore(c, false)

	cfg, err := cmd.Config(s.store, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cfg.Name(), gc.Equals, s.envName)
	c.Check(s.client.getCalled, jc.IsTrue)
	c.Check(s.client.closeCalled, jc.IsTrue)
}

func (s *EnvConfigSuite) TestConfigWithNoBootstrapWithClientErr(c *gc.C) {
	cmd := envcmd.NewEnvCommandBase(s.envName, s.client, errors.New("problem opening connection"))
	s.writeStore(c, false)

	_, err := cmd.Config(s.store, nil)
	c.Assert(err, gc.ErrorMatches, "problem opening connection")
	c.Check(s.client.getCalled, jc.IsFalse)
	c.Check(s.client.closeCalled, jc.IsFalse)
}

func (s *EnvConfigSuite) TestConfigWithNoBootstrapWithEnvGetError(c *gc.C) {
	cmd := envcmd.NewEnvCommandBase(s.envName, s.client, nil)
	s.writeStore(c, false)
	s.client.err = errors.New("problem getting environment attributes")

	_, err := cmd.Config(s.store, nil)
	c.Assert(err, gc.ErrorMatches, "problem getting environment attributes")
	c.Check(s.client.getCalled, jc.IsTrue)
	c.Check(s.client.closeCalled, jc.IsTrue)
}

func (s *EnvConfigSuite) TestConfigEnvDoesntExist(c *gc.C) {
	cmd := envcmd.NewEnvCommandBase("dummy", s.client, nil)
	s.writeStore(c, false)

	_, err := cmd.Config(s.store, nil)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Check(s.client.getCalled, jc.IsFalse)
	c.Check(s.client.closeCalled, jc.IsFalse)
}
