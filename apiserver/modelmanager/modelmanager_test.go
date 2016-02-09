// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager_test

import (
	"github.com/juju/loggo"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/modelmanager"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	jujutesting "github.com/juju/juju/juju/testing"
	// Register the providers for the field check test
	_ "github.com/juju/juju/provider/azure"
	_ "github.com/juju/juju/provider/ec2"
	_ "github.com/juju/juju/provider/joyent"
	_ "github.com/juju/juju/provider/maas"
	_ "github.com/juju/juju/provider/openstack"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type modelManagerBaseSuite struct {
	jujutesting.JujuConnSuite

	modelmanager *modelmanager.ModelManagerAPI
	resources    *common.Resources
	authoriser   apiservertesting.FakeAuthorizer
}

func (s *modelManagerBaseSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	s.authoriser = apiservertesting.FakeAuthorizer{
		Tag: s.AdminUserTag(c),
	}

	loggo.GetLogger("juju.apiserver.modelmanager").SetLogLevel(loggo.TRACE)
}

func (s *modelManagerBaseSuite) setAPIUser(c *gc.C, user names.UserTag) {
	s.authoriser.Tag = user
	modelmanager, err := modelmanager.NewModelManagerAPI(s.State, s.resources, s.authoriser)
	c.Assert(err, jc.ErrorIsNil)
	s.modelmanager = modelmanager
}

type modelManagerSuite struct {
	modelManagerBaseSuite
}

var _ = gc.Suite(&modelManagerSuite{})

func (s *modelManagerSuite) TestNewAPIAcceptsClient(c *gc.C) {
	anAuthoriser := s.authoriser
	anAuthoriser.Tag = names.NewUserTag("external@remote")
	endPoint, err := modelmanager.NewModelManagerAPI(s.State, s.resources, anAuthoriser)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(endPoint, gc.NotNil)
}

func (s *modelManagerSuite) TestNewAPIRefusesNonClient(c *gc.C) {
	anAuthoriser := s.authoriser
	anAuthoriser.Tag = names.NewUnitTag("mysql/0")
	endPoint, err := modelmanager.NewModelManagerAPI(s.State, s.resources, anAuthoriser)
	c.Assert(endPoint, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *modelManagerSuite) createArgs(c *gc.C, owner names.UserTag) params.ModelCreateArgs {
	return params.ModelCreateArgs{
		OwnerTag: owner.String(),
		Account:  make(map[string]interface{}),
		Config: map[string]interface{}{
			"name":            "test-model",
			"authorized-keys": "ssh-key",
			// And to make it a valid dummy config
			"controller": false,
		},
	}
}

func (s *modelManagerSuite) createArgsForVersion(c *gc.C, owner names.UserTag, ver interface{}) params.ModelCreateArgs {
	params := s.createArgs(c, owner)
	params.Config["agent-version"] = ver
	return params
}

func (s *modelManagerSuite) TestUserCanCreateModel(c *gc.C) {
	owner := names.NewUserTag("external@remote")
	s.setAPIUser(c, owner)
	model, err := s.modelmanager.CreateModel(s.createArgs(c, owner))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.OwnerTag, gc.Equals, owner.String())
	c.Assert(model.Name, gc.Equals, "test-model")
}

func (s *modelManagerSuite) TestAdminCanCreateModelForSomeoneElse(c *gc.C) {
	s.setAPIUser(c, s.AdminUserTag(c))
	owner := names.NewUserTag("external@remote")
	model, err := s.modelmanager.CreateModel(s.createArgs(c, owner))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.OwnerTag, gc.Equals, owner.String())
	c.Assert(model.Name, gc.Equals, "test-model")
	// Make sure that the environment created does actually have the correct
	// owner, and that owner is actually allowed to use the environment.
	newState, err := s.State.ForModel(names.NewModelTag(model.UUID))
	c.Assert(err, jc.ErrorIsNil)
	defer newState.Close()

	newModel, err := newState.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newModel.Owner(), gc.Equals, owner)
	_, err = newState.ModelUser(owner)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *modelManagerSuite) TestNonAdminCannotCreateModelForSomeoneElse(c *gc.C) {
	s.setAPIUser(c, names.NewUserTag("non-admin@remote"))
	owner := names.NewUserTag("external@remote")
	_, err := s.modelmanager.CreateModel(s.createArgs(c, owner))
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *modelManagerSuite) TestRestrictedProviderFields(c *gc.C) {
	s.setAPIUser(c, names.NewUserTag("non-admin@remote"))
	for i, test := range []struct {
		provider string
		expected []string
	}{
		{
			provider: "azure",
			expected: []string{
				"type", "ca-cert", "state-port", "api-port",
				"subscription-id", "tenant-id", "application-id", "application-password", "location",
				"controller-resource-group", "storage-account-type"},
		}, {
			provider: "dummy",
			expected: []string{
				"type", "ca-cert", "state-port", "api-port"},
		}, {
			provider: "joyent",
			expected: []string{
				"type", "ca-cert", "state-port", "api-port"},
		}, {
			provider: "maas",
			expected: []string{
				"type", "ca-cert", "state-port", "api-port",
				"maas-server"},
		}, {
			provider: "openstack",
			expected: []string{
				"type", "ca-cert", "state-port", "api-port",
				"region", "auth-url", "auth-mode"},
		}, {
			provider: "ec2",
			expected: []string{
				"type", "ca-cert", "state-port", "api-port",
				"region"},
		},
	} {
		c.Logf("%d: %s provider", i, test.provider)
		fields, err := modelmanager.RestrictedProviderFields(s.modelmanager, test.provider)
		c.Check(err, jc.ErrorIsNil)
		c.Check(fields, jc.SameContents, test.expected)
	}
}

func (s *modelManagerSuite) TestConfigSkeleton(c *gc.C) {
	s.setAPIUser(c, names.NewUserTag("non-admin@remote"))

	_, err := s.modelmanager.ConfigSkeleton(
		params.ModelSkeletonConfigArgs{Provider: "ec2"})
	c.Check(err, gc.ErrorMatches, `provider value "ec2" not valid`)
	_, err = s.modelmanager.ConfigSkeleton(
		params.ModelSkeletonConfigArgs{Region: "the sun"})
	c.Check(err, gc.ErrorMatches, `region value "the sun" not valid`)

	skeleton, err := s.modelmanager.ConfigSkeleton(params.ModelSkeletonConfigArgs{})
	c.Assert(err, jc.ErrorIsNil)

	// The apiPort changes every test run as the dummy provider
	// looks for a random open port.
	apiPort := s.Environ.Config().APIPort()

	c.Assert(skeleton.Config, jc.DeepEquals, params.ModelConfig{
		"type":       "dummy",
		"ca-cert":    coretesting.CACert,
		"state-port": 1234,
		"api-port":   apiPort,
	})
}

func (s *modelManagerSuite) TestCreateModelValidatesConfig(c *gc.C) {
	admin := s.AdminUserTag(c)
	s.setAPIUser(c, admin)
	args := s.createArgs(c, admin)
	args.Config["controller"] = "maybe"
	_, err := s.modelmanager.CreateModel(args)
	c.Assert(err, gc.ErrorMatches, "provider validation failed: controller: expected bool, got string\\(\"maybe\"\\)")
}

func (s *modelManagerSuite) TestCreateModelBadConfig(c *gc.C) {
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
		},
	} {
		c.Logf("%d: %s", i, test.key)
		args := s.createArgs(c, owner)
		args.Config[test.key] = test.value
		_, err := s.modelmanager.CreateModel(args)
		c.Assert(err, gc.ErrorMatches, test.errMatch)

	}
}

func (s *modelManagerSuite) TestCreateModelSameAgentVersion(c *gc.C) {
	admin := s.AdminUserTag(c)
	s.setAPIUser(c, admin)
	args := s.createArgsForVersion(c, admin, version.Current.String())
	_, err := s.modelmanager.CreateModel(args)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *modelManagerSuite) TestCreateModelBadAgentVersion(c *gc.C) {
	s.PatchValue(&version.Current, coretesting.FakeVersionNumber)
	admin := s.AdminUserTag(c)
	s.setAPIUser(c, admin)

	bigger := version.Current
	bigger.Minor += 1

	smaller := version.Current
	smaller.Minor -= 1

	for i, test := range []struct {
		value    interface{}
		errMatch string
	}{
		{
			value:    42,
			errMatch: `failed to create config: agent-version must be a string but has type 'int'`,
		}, {
			value:    "not a number",
			errMatch: `failed to create config: invalid version \"not a number\"`,
		}, {
			value:    bigger.String(),
			errMatch: "failed to create config: agent-version cannot be greater than the server: .*",
		}, {
			value:    smaller.String(),
			errMatch: "failed to create config: no tools found for version .*",
		},
	} {
		c.Logf("test %d", i)
		args := s.createArgsForVersion(c, admin, test.value)
		_, err := s.modelmanager.CreateModel(args)
		c.Check(err, gc.ErrorMatches, test.errMatch)
	}
}

func (s *modelManagerSuite) TestListModelsForSelf(c *gc.C) {
	user := names.NewUserTag("external@remote")
	s.setAPIUser(c, user)
	result, err := s.modelmanager.ListModels(params.Entity{user.String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.UserModels, gc.HasLen, 0)
}

func (s *modelManagerSuite) TestListModelsForSelfLocalUser(c *gc.C) {
	// When the user's credentials cache stores the simple name, but the
	// api server converts it to a fully qualified name.
	user := names.NewUserTag("local-user")
	s.setAPIUser(c, names.NewUserTag("local-user@local"))
	result, err := s.modelmanager.ListModels(params.Entity{user.String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.UserModels, gc.HasLen, 0)
}

func (s *modelManagerSuite) checkModelMatches(c *gc.C, env params.Model, expected *state.Model) {
	c.Check(env.Name, gc.Equals, expected.Name())
	c.Check(env.UUID, gc.Equals, expected.UUID())
	c.Check(env.OwnerTag, gc.Equals, expected.Owner().String())
}

func (s *modelManagerSuite) TestListModelsAdminSelf(c *gc.C) {
	user := s.AdminUserTag(c)
	s.setAPIUser(c, user)
	result, err := s.modelmanager.ListModels(params.Entity{user.String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.UserModels, gc.HasLen, 1)
	expected, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	s.checkModelMatches(c, result.UserModels[0].Model, expected)
}

func (s *modelManagerSuite) TestListModelsAdminListsOther(c *gc.C) {
	user := s.AdminUserTag(c)
	s.setAPIUser(c, user)
	other := names.NewUserTag("external@remote")
	result, err := s.modelmanager.ListModels(params.Entity{other.String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.UserModels, gc.HasLen, 0)
}

func (s *modelManagerSuite) TestListModelsDenied(c *gc.C) {
	user := names.NewUserTag("external@remote")
	s.setAPIUser(c, user)
	other := names.NewUserTag("other@remote")
	_, err := s.modelmanager.ListModels(params.Entity{other.String()})
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
