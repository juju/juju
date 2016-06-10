// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller/modelmanager"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	_ "github.com/juju/juju/provider/all"
	_ "github.com/juju/juju/provider/azure"
	_ "github.com/juju/juju/provider/dummy"
	_ "github.com/juju/juju/provider/ec2"
	_ "github.com/juju/juju/provider/joyent"
	_ "github.com/juju/juju/provider/maas"
	_ "github.com/juju/juju/provider/openstack"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
)

type ModelConfigCreatorSuite struct {
	coretesting.BaseSuite
	creator    modelmanager.ModelConfigCreator
	baseConfig *config.Config
}

var _ = gc.Suite(&ModelConfigCreatorSuite{})

func (s *ModelConfigCreatorSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.creator = modelmanager.ModelConfigCreator{}
	baseConfig, err := config.New(
		config.UseDefaults,
		coretesting.FakeConfig().Merge(coretesting.Attrs{
			"type":          "fake",
			"restricted":    "area51",
			"agent-version": "2.0.0",
		}),
	)
	c.Assert(err, jc.ErrorIsNil)
	s.baseConfig = baseConfig
	fake.Reset()
}

func (s *ModelConfigCreatorSuite) newModelConfig(attrs map[string]interface{}) (*config.Config, error) {
	return s.creator.NewModelConfig(modelmanager.IsNotAdmin, s.baseConfig, attrs)
}

func (s *ModelConfigCreatorSuite) newModelConfigAdmin(attrs map[string]interface{}) (*config.Config, error) {
	return s.creator.NewModelConfig(modelmanager.IsAdmin, s.baseConfig, attrs)
}

func (s *ModelConfigCreatorSuite) TestCreateModelValidatesConfig(c *gc.C) {
	newModelUUID := utils.MustNewUUID().String()
	cfg, err := s.newModelConfig(coretesting.Attrs(
		s.baseConfig.AllAttrs(),
	).Merge(coretesting.Attrs{
		"name":       "new-model",
		"additional": "value",
		"uuid":       newModelUUID,
	}))
	c.Assert(err, jc.ErrorIsNil)
	expected := s.baseConfig.AllAttrs()
	expected["name"] = "new-model"
	expected["additional"] = "value"
	expected["uuid"] = newModelUUID
	c.Assert(cfg.AllAttrs(), jc.DeepEquals, expected)

	fake.Stub.CheckCallNames(c,
		"RestrictedConfigAttributes",
		"PrepareForCreateEnvironment",
		"Validate",
	)
	validateCall := fake.Stub.Calls()[2]
	c.Assert(validateCall.Args, gc.HasLen, 2)
	c.Assert(validateCall.Args[0], gc.Equals, cfg)
	c.Assert(validateCall.Args[1], gc.IsNil)
}

func (s *ModelConfigCreatorSuite) TestCreateModelForAdminUserCopiesSecrets(c *gc.C) {
	var err error
	s.baseConfig, err = s.baseConfig.Apply(coretesting.Attrs{
		"username":        "user",
		"password":        "password",
		"authorized-keys": "ssh-key",
	})
	c.Assert(err, jc.ErrorIsNil)
	newModelUUID := utils.MustNewUUID().String()
	newAttrs := coretesting.Attrs{
		"name":       "new-model",
		"additional": "value",
		"uuid":       newModelUUID,
	}
	cfg, err := s.newModelConfigAdmin(newAttrs)
	c.Assert(err, jc.ErrorIsNil)
	expectedCfg, err := config.New(config.UseDefaults, newAttrs)
	c.Assert(err, jc.ErrorIsNil)
	expected := expectedCfg.AllAttrs()
	c.Assert(expected["username"], gc.Equals, "user")
	c.Assert(expected["password"], gc.Equals, "password")
	c.Assert(expected["authorized-keys"], gc.Equals, "ssh-key")
	c.Assert(cfg.AllAttrs(), jc.DeepEquals, expected)

	fake.Stub.CheckCallNames(c,
		"RestrictedConfigAttributes",
		"PrepareForCreateEnvironment",
		"Validate",
	)
	validateCall := fake.Stub.Calls()[2]
	c.Assert(validateCall.Args, gc.HasLen, 2)
	c.Assert(validateCall.Args[0], gc.Equals, cfg)
	c.Assert(validateCall.Args[1], gc.IsNil)
}

func (s *ModelConfigCreatorSuite) TestCreateModelEnsuresRequiredFields(c *gc.C) {
	var err error
	s.baseConfig, err = s.baseConfig.Apply(coretesting.Attrs{
		"authorized-keys": "ssh-key",
	})
	c.Assert(err, jc.ErrorIsNil)
	newAttrs := coretesting.Attrs{
		"name": "new-model",
	}
	_, err = s.newModelConfigAdmin(newAttrs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newAttrs["authorized-keys"], gc.Equals, "ssh-key")
}

func (s *ModelConfigCreatorSuite) TestCreateModelForAdminUserPrefersUserSecrets(c *gc.C) {
	var err error
	s.baseConfig, err = s.baseConfig.Apply(coretesting.Attrs{
		"username":        "user",
		"password":        "password",
		"authorized-keys": "ssh-key",
	})
	c.Assert(err, jc.ErrorIsNil)
	newModelUUID := utils.MustNewUUID().String()
	newAttrs := coretesting.Attrs{
		"name":       "new-model",
		"additional": "value",
		"uuid":       newModelUUID,
		"username":   "anotheruser",
		"password":   "anotherpassword",
	}
	cfg, err := s.newModelConfigAdmin(newAttrs)
	c.Assert(err, jc.ErrorIsNil)
	expectedCfg, err := config.New(config.UseDefaults, newAttrs)
	c.Assert(err, jc.ErrorIsNil)
	expected := expectedCfg.AllAttrs()
	c.Assert(expected["username"], gc.Equals, "anotheruser")
	c.Assert(expected["password"], gc.Equals, "anotherpassword")
	c.Assert(expected["authorized-keys"], gc.Equals, "ssh-key")
	c.Assert(cfg.AllAttrs(), jc.DeepEquals, expected)

	fake.Stub.CheckCallNames(c,
		"RestrictedConfigAttributes",
		"PrepareForCreateEnvironment",
		"Validate",
	)
	validateCall := fake.Stub.Calls()[2]
	c.Assert(validateCall.Args, gc.HasLen, 2)
	c.Assert(validateCall.Args[0], gc.Equals, cfg)
	c.Assert(validateCall.Args[1], gc.IsNil)
}

func (s *ModelConfigCreatorSuite) TestCreateModelBadConfig(c *gc.C) {
	for i, test := range []struct {
		key      string
		value    interface{}
		errMatch string
	}{{
		key:      "type",
		value:    "dummy",
		errMatch: `specified type "dummy" does not match controller "fake"`,
	}, {
		key:      "restricted",
		value:    51,
		errMatch: `specified restricted "51" does not match controller "area51"`,
	}} {
		c.Logf("%d: %s", i, test.key)
		_, err := s.newModelConfig(coretesting.Attrs(
			s.baseConfig.AllAttrs(),
		).Merge(coretesting.Attrs{
			test.key: test.value,
		}))
		c.Check(err, gc.ErrorMatches, test.errMatch)
	}
}

func (s *ModelConfigCreatorSuite) TestCreateModelSameAgentVersion(c *gc.C) {
	cfg, err := s.newModelConfig(coretesting.Attrs(
		s.baseConfig.AllAttrs(),
	).Merge(coretesting.Attrs{
		"name": "new-model",
		"uuid": utils.MustNewUUID().String(),
	}))
	c.Assert(err, jc.ErrorIsNil)

	baseAgentVersion, ok := s.baseConfig.AgentVersion()
	c.Assert(ok, jc.IsTrue)
	agentVersion, ok := cfg.AgentVersion()
	c.Assert(ok, jc.IsTrue)
	c.Assert(agentVersion, gc.Equals, baseAgentVersion)
}

func (s *ModelConfigCreatorSuite) TestCreateModelGreaterAgentVersion(c *gc.C) {
	_, err := s.newModelConfig(coretesting.Attrs(
		s.baseConfig.AllAttrs(),
	).Merge(coretesting.Attrs{
		"name":          "new-model",
		"uuid":          utils.MustNewUUID().String(),
		"agent-version": "2.0.1",
	}))
	c.Assert(err, gc.ErrorMatches,
		"agent-version .* cannot be greater than the controller .*")
}

func (s *ModelConfigCreatorSuite) TestCreateModelLesserAgentVersionNoToolsFinder(c *gc.C) {
	_, err := s.newModelConfig(coretesting.Attrs(
		s.baseConfig.AllAttrs(),
	).Merge(coretesting.Attrs{
		"name":          "new-model",
		"uuid":          utils.MustNewUUID().String(),
		"agent-version": "1.9.9",
	}))
	c.Assert(err, gc.ErrorMatches,
		"agent-version does not match base config, and no tools-finder is supplied")
}

func (s *ModelConfigCreatorSuite) TestCreateModelLesserAgentVersionToolsFinderFound(c *gc.C) {
	s.creator.FindTools = func(version.Number) (tools.List, error) {
		return tools.List{
			{}, //contents don't matter, just need a non-empty list
		}, nil
	}
	cfg, err := s.newModelConfig(coretesting.Attrs(
		s.baseConfig.AllAttrs(),
	).Merge(coretesting.Attrs{
		"name":          "new-model",
		"uuid":          utils.MustNewUUID().String(),
		"agent-version": "1.9.9",
	}))
	c.Assert(err, jc.ErrorIsNil)
	agentVersion, ok := cfg.AgentVersion()
	c.Assert(ok, jc.IsTrue)
	c.Assert(agentVersion, gc.Equals, version.MustParse("1.9.9"))
}

func (s *ModelConfigCreatorSuite) TestCreateModelLesserAgentVersionToolsFinderNotFound(c *gc.C) {
	s.creator.FindTools = func(version.Number) (tools.List, error) {
		return tools.List{}, nil
	}
	_, err := s.newModelConfig(coretesting.Attrs(
		s.baseConfig.AllAttrs(),
	).Merge(coretesting.Attrs{
		"name":          "new-model",
		"uuid":          utils.MustNewUUID().String(),
		"agent-version": "1.9.9",
	}))
	c.Assert(err, gc.ErrorMatches, "no tools found for version .*")
}

type RestrictedProviderFieldsSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&RestrictedProviderFieldsSuite{})

func (*RestrictedProviderFieldsSuite) TestRestrictedProviderFields(c *gc.C) {
	for i, test := range []struct {
		provider string
		expected []string
	}{{
		provider: "azure",
		expected: []string{
			"type",
			"location", "endpoint", "storage-endpoint",
		},
	}, {
		provider: "dummy",
		expected: []string{
			"type",
		},
	}, {
		provider: "joyent",
		expected: []string{
			"type", "sdc-url",
		},
	}, {
		provider: "maas",
		expected: []string{
			"type",
			"maas-server",
		},
	}, {
		provider: "openstack",
		expected: []string{
			"type",
			"region", "auth-url", "auth-mode",
		},
	}, {
		provider: "ec2",
		expected: []string{
			"type",
			"region", "vpc-id-force",
		},
	}} {
		c.Logf("%d: %s provider", i, test.provider)
		fields, err := modelmanager.RestrictedProviderFields(test.provider)
		c.Check(err, jc.ErrorIsNil)
		c.Check(fields, jc.SameContents, test.expected)
	}
}

type fakeProvider struct {
	testing.Stub
	environs.EnvironProvider
	restrictedConfigAttributes []string
}

func (p *fakeProvider) Reset() {
	p.Stub.ResetCalls()
	p.restrictedConfigAttributes = []string{"restricted"}
}

func (p *fakeProvider) RestrictedConfigAttributes() []string {
	p.MethodCall(p, "RestrictedConfigAttributes")
	return p.restrictedConfigAttributes
}

func (p *fakeProvider) Validate(cfg, old *config.Config) (*config.Config, error) {
	p.MethodCall(p, "Validate", cfg, old)
	return cfg, p.NextErr()
}

func (p *fakeProvider) PrepareForCreateEnvironment(cfg *config.Config) (*config.Config, error) {
	p.MethodCall(p, "PrepareForCreateEnvironment", cfg)
	return cfg, p.NextErr()
}

func (p *fakeProvider) CredentialSchemas() map[cloud.AuthType]cloud.CredentialSchema {
	return map[cloud.AuthType]cloud.CredentialSchema{
		cloud.UserPassAuthType: {
			{
				"username", cloud.CredentialAttr{Description: "The username"},
			}, {
				"password", cloud.CredentialAttr{
					Description: "The password",
					Hidden:      true,
				},
			},
		},
	}
}

func (p *fakeProvider) DetectCredentials() (*cloud.CloudCredential, error) {
	return nil, errors.NotFoundf("credentials")
}

var fake fakeProvider

func init() {
	fake.Reset()
	environs.RegisterProvider("fake", &fake)
}
