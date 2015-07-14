// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environmentmanager_test

import (
	"github.com/juju/loggo"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/environmentmanager"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	jujutesting "github.com/juju/juju/juju/testing"
	// Register the providers for the field check test
	_ "github.com/juju/juju/provider/azure"
	_ "github.com/juju/juju/provider/ec2"
	_ "github.com/juju/juju/provider/joyent"
	_ "github.com/juju/juju/provider/local"
	_ "github.com/juju/juju/provider/maas"
	_ "github.com/juju/juju/provider/openstack"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type envManagerBaseSuite struct {
	jujutesting.JujuConnSuite

	envmanager *environmentmanager.EnvironmentManagerAPI
	resources  *common.Resources
	authoriser apiservertesting.FakeAuthorizer
}

func (s *envManagerBaseSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	s.authoriser = apiservertesting.FakeAuthorizer{
		Tag: s.AdminUserTag(c),
	}

	loggo.GetLogger("juju.apiserver.environmentmanager").SetLogLevel(loggo.TRACE)
}

func (s *envManagerBaseSuite) setAPIUser(c *gc.C, user names.UserTag) {
	s.authoriser.Tag = user
	envmanager, err := environmentmanager.NewEnvironmentManagerAPI(s.State, s.resources, s.authoriser)
	c.Assert(err, jc.ErrorIsNil)
	s.envmanager = envmanager
}

type envManagerSuite struct {
	envManagerBaseSuite
}

var _ = gc.Suite(&envManagerSuite{})

func (s *envManagerSuite) TestNewAPIAcceptsClient(c *gc.C) {
	anAuthoriser := s.authoriser
	anAuthoriser.Tag = names.NewUserTag("external@remote")
	endPoint, err := environmentmanager.NewEnvironmentManagerAPI(s.State, s.resources, anAuthoriser)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(endPoint, gc.NotNil)
}

func (s *envManagerSuite) TestNewAPIRefusesNonClient(c *gc.C) {
	anAuthoriser := s.authoriser
	anAuthoriser.Tag = names.NewUnitTag("mysql/0")
	endPoint, err := environmentmanager.NewEnvironmentManagerAPI(s.State, s.resources, anAuthoriser)
	c.Assert(endPoint, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *envManagerSuite) createArgs(c *gc.C, owner names.UserTag) params.EnvironmentCreateArgs {
	return params.EnvironmentCreateArgs{
		OwnerTag: owner.String(),
		Account:  make(map[string]interface{}),
		Config: map[string]interface{}{
			"name":            "test-env",
			"authorized-keys": "ssh-key",
			// And to make it a valid dummy config
			"state-server": false,
		},
	}
}

func (s *envManagerSuite) createArgsForVersion(c *gc.C, owner names.UserTag, ver interface{}) params.EnvironmentCreateArgs {
	params := s.createArgs(c, owner)
	params.Config["agent-version"] = ver
	return params
}

func (s *envManagerSuite) TestUserCanCreateEnvironment(c *gc.C) {
	owner := names.NewUserTag("external@remote")
	s.setAPIUser(c, owner)
	env, err := s.envmanager.CreateEnvironment(s.createArgs(c, owner))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.OwnerTag, gc.Equals, owner.String())
	c.Assert(env.Name, gc.Equals, "test-env")
}

func (s *envManagerSuite) TestAdminCanCreateEnvironmentForSomeoneElse(c *gc.C) {
	s.setAPIUser(c, s.AdminUserTag(c))
	owner := names.NewUserTag("external@remote")
	env, err := s.envmanager.CreateEnvironment(s.createArgs(c, owner))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.OwnerTag, gc.Equals, owner.String())
	c.Assert(env.Name, gc.Equals, "test-env")
	// Make sure that the environment created does actually have the correct
	// owner, and that owner is actually allowed to use the environment.
	newState, err := s.State.ForEnviron(names.NewEnvironTag(env.UUID))
	c.Assert(err, jc.ErrorIsNil)
	defer newState.Close()

	newEnv, err := newState.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newEnv.Owner(), gc.Equals, owner)
	_, err = newState.EnvironmentUser(owner)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *envManagerSuite) TestNonAdminCannotCreateEnvironmentForSomeoneElse(c *gc.C) {
	s.setAPIUser(c, names.NewUserTag("non-admin@remote"))
	owner := names.NewUserTag("external@remote")
	_, err := s.envmanager.CreateEnvironment(s.createArgs(c, owner))
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *envManagerSuite) TestRestrictedProviderFields(c *gc.C) {
	s.setAPIUser(c, names.NewUserTag("non-admin@remote"))
	for i, test := range []struct {
		provider string
		expected []string
	}{
		{
			provider: "azure",
			expected: []string{
				"type", "ca-cert", "state-port", "api-port", "syslog-port", "rsyslog-ca-cert", "rsyslog-ca-key",
				"location"},
		}, {
			provider: "dummy",
			expected: []string{
				"type", "ca-cert", "state-port", "api-port", "syslog-port", "rsyslog-ca-cert", "rsyslog-ca-key"},
		}, {
			provider: "joyent",
			expected: []string{
				"type", "ca-cert", "state-port", "api-port", "syslog-port", "rsyslog-ca-cert", "rsyslog-ca-key"},
		}, {
			provider: "local",
			expected: []string{
				"type", "ca-cert", "state-port", "api-port", "syslog-port", "rsyslog-ca-cert", "rsyslog-ca-key",
				"container", "network-bridge", "root-dir", "proxy-ssh"},
		}, {
			provider: "maas",
			expected: []string{
				"type", "ca-cert", "state-port", "api-port", "syslog-port", "rsyslog-ca-cert", "rsyslog-ca-key",
				"maas-server"},
		}, {
			provider: "openstack",
			expected: []string{
				"type", "ca-cert", "state-port", "api-port", "syslog-port", "rsyslog-ca-cert", "rsyslog-ca-key",
				"region", "auth-url", "auth-mode"},
		}, {
			provider: "ec2",
			expected: []string{
				"type", "ca-cert", "state-port", "api-port", "syslog-port", "rsyslog-ca-cert", "rsyslog-ca-key",
				"region"},
		},
	} {
		c.Logf("%d: %s provider", i, test.provider)
		fields, err := environmentmanager.RestrictedProviderFields(s.envmanager, test.provider)
		c.Check(err, jc.ErrorIsNil)
		c.Check(fields, jc.SameContents, test.expected)
	}
}

func (s *envManagerSuite) TestConfigSkeleton(c *gc.C) {
	s.setAPIUser(c, names.NewUserTag("non-admin@remote"))

	_, err := s.envmanager.ConfigSkeleton(
		params.EnvironmentSkeletonConfigArgs{Provider: "ec2"})
	c.Check(err, gc.ErrorMatches, `provider value "ec2" not valid`)
	_, err = s.envmanager.ConfigSkeleton(
		params.EnvironmentSkeletonConfigArgs{Region: "the sun"})
	c.Check(err, gc.ErrorMatches, `region value "the sun" not valid`)

	skeleton, err := s.envmanager.ConfigSkeleton(params.EnvironmentSkeletonConfigArgs{})
	c.Assert(err, jc.ErrorIsNil)

	// The apiPort changes every test run as the dummy provider
	// looks for a random open port.
	apiPort := s.Environ.Config().APIPort()

	c.Assert(skeleton.Config, jc.DeepEquals, params.EnvironConfig{
		"type":        "dummy",
		"ca-cert":     coretesting.CACert,
		"state-port":  1234,
		"api-port":    apiPort,
		"syslog-port": 2345,
	})
}

func (s *envManagerSuite) TestCreateEnvironmentValidatesConfig(c *gc.C) {
	admin := s.AdminUserTag(c)
	s.setAPIUser(c, admin)
	args := s.createArgs(c, admin)
	delete(args.Config, "state-server")
	_, err := s.envmanager.CreateEnvironment(args)
	c.Assert(err, gc.ErrorMatches, "provider validation failed: state-server: expected bool, got nothing")
}

func (s *envManagerSuite) TestCreateEnvironmentBadConfig(c *gc.C) {
	owner := names.NewUserTag("external@remote")
	s.setAPIUser(c, owner)
	for i, test := range []struct {
		key      string
		value    interface{}
		errMatch string
	}{
		{
			key:      "uuid",
			value:    "anything",
			errMatch: `uuid is generated, you cannot specify one`,
		}, {
			key:      "type",
			value:    "fake",
			errMatch: `specified type "fake" does not match apiserver "dummy"`,
		}, {
			key:      "ca-cert",
			value:    coretesting.OtherCACert,
			errMatch: `(?s)specified ca-cert ".*" does not match apiserver ".*"`,
		}, {
			key:      "state-port",
			value:    9876,
			errMatch: `specified state-port "9876" does not match apiserver "1234"`,
		}, {
			// The api-port is dynamic, but always in user-space, so > 1024.
			key:      "api-port",
			value:    123,
			errMatch: `specified api-port "123" does not match apiserver ".*"`,
		}, {
			key:      "syslog-port",
			value:    1234,
			errMatch: `specified syslog-port "1234" does not match apiserver "2345"`,
		}, {
			key:      "rsyslog-ca-cert",
			value:    "some-cert",
			errMatch: `specified rsyslog-ca-cert "some-cert" does not match apiserver ".*"`,
		}, {
			key:      "rsyslog-ca-key",
			value:    "some-key",
			errMatch: `specified rsyslog-ca-key "some-key" does not match apiserver ".*"`,
		},
	} {
		c.Logf("%d: %s", i, test.key)
		args := s.createArgs(c, owner)
		args.Config[test.key] = test.value
		_, err := s.envmanager.CreateEnvironment(args)
		c.Assert(err, gc.ErrorMatches, test.errMatch)

	}
}

func (s *envManagerSuite) TestCreateEnvironmentSameAgentVersion(c *gc.C) {
	admin := s.AdminUserTag(c)
	s.setAPIUser(c, admin)
	args := s.createArgsForVersion(c, admin, version.Current.Number.String())
	_, err := s.envmanager.CreateEnvironment(args)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *envManagerSuite) TestCreateEnvironmentBadAgentVersion(c *gc.C) {
	admin := s.AdminUserTag(c)
	s.setAPIUser(c, admin)

	bigger := version.Current.Number
	bigger.Minor += 1

	smaller := version.Current.Number
	smaller.Minor -= 1

	for i, test := range []struct {
		value    interface{}
		errMatch string
	}{
		{
			value:    42,
			errMatch: `creating config from values failed: agent-version: expected string, got int\(42\)`,
		}, {
			value:    "not a number",
			errMatch: `creating config from values failed: invalid agent version in environment configuration: "not a number"`,
		}, {
			value:    bigger.String(),
			errMatch: "agent-version cannot be greater than the server: .*",
		}, {
			value:    smaller.String(),
			errMatch: "no tools found for version .*",
		},
	} {
		c.Logf("test %d", i)
		args := s.createArgsForVersion(c, admin, test.value)
		_, err := s.envmanager.CreateEnvironment(args)
		c.Check(err, gc.ErrorMatches, test.errMatch)
	}
}

func (s *envManagerSuite) TestListEnvironmentsForSelf(c *gc.C) {
	user := names.NewUserTag("external@remote")
	s.setAPIUser(c, user)
	result, err := s.envmanager.ListEnvironments(params.Entity{user.String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.UserEnvironments, gc.HasLen, 0)
}

func (s *envManagerSuite) TestListEnvironmentsForSelfLocalUser(c *gc.C) {
	// When the user's credentials cache stores the simple name, but the
	// api server converts it to a fully qualified name.
	user := names.NewUserTag("local-user")
	s.setAPIUser(c, names.NewUserTag("local-user@local"))
	result, err := s.envmanager.ListEnvironments(params.Entity{user.String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.UserEnvironments, gc.HasLen, 0)
}

func (s *envManagerSuite) checkEnvironmentMatches(c *gc.C, env params.Environment, expected *state.Environment) {
	c.Check(env.Name, gc.Equals, expected.Name())
	c.Check(env.UUID, gc.Equals, expected.UUID())
	c.Check(env.OwnerTag, gc.Equals, expected.Owner().String())
}

func (s *envManagerSuite) TestListEnvironmentsAdminSelf(c *gc.C) {
	user := s.AdminUserTag(c)
	s.setAPIUser(c, user)
	result, err := s.envmanager.ListEnvironments(params.Entity{user.String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.UserEnvironments, gc.HasLen, 1)
	expected, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	s.checkEnvironmentMatches(c, result.UserEnvironments[0].Environment, expected)
}

func (s *envManagerSuite) TestListEnvironmentsAdminListsOther(c *gc.C) {
	user := s.AdminUserTag(c)
	s.setAPIUser(c, user)
	other := names.NewUserTag("external@remote")
	result, err := s.envmanager.ListEnvironments(params.Entity{other.String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.UserEnvironments, gc.HasLen, 0)
}

func (s *envManagerSuite) TestListEnvironmentsDenied(c *gc.C) {
	user := names.NewUserTag("external@remote")
	s.setAPIUser(c, user)
	other := names.NewUserTag("other@remote")
	_, err := s.envmanager.ListEnvironments(params.Entity{other.String()})
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

type fakeProvider struct {
	environs.EnvironProvider
}

func (*fakeProvider) Validate(cfg, old *config.Config) (*config.Config, error) {
	return cfg, nil
}

func (*fakeProvider) PrepareForCreateEnvironment(cfg *config.Config) (*config.Config, error) {
	return cfg, nil
}

func init() {
	environs.RegisterProvider("fake", &fakeProvider{})
}
