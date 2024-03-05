// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller/modelmanager"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/tools"
	"github.com/juju/juju/internal/uuid"
	coretesting "github.com/juju/juju/testing"
)

type ModelConfigCreatorSuite struct {
	coretesting.BaseSuite
	fake       fakeProvider
	creator    modelmanager.ModelConfigCreator
	baseConfig *config.Config
}

var _ = gc.Suite(&ModelConfigCreatorSuite{})

func (s *ModelConfigCreatorSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.fake = fakeProvider{
		restrictedConfigAttributes: []string{"restricted"},
	}
	s.creator = modelmanager.ModelConfigCreator{
		Provider: func(provider string) (environs.EnvironProvider, error) {
			if provider != "fake" {
				return nil, errors.Errorf("expected fake, got %s", provider)
			}
			return &s.fake, nil
		},
	}
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
}

func (s *ModelConfigCreatorSuite) newModelConfig(attrs map[string]interface{}) (*config.Config, error) {
	cloudSpec := environscloudspec.CloudSpec{Type: "fake"}
	return s.creator.NewModelConfig(cloudSpec, s.baseConfig, attrs)
}

func (s *ModelConfigCreatorSuite) TestCreateModelValidatesConfig(c *gc.C) {
	newModelUUID := uuid.MustNewUUID().String()
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
	expected["type"] = "fake"
	expected["additional"] = "value"
	expected["uuid"] = newModelUUID
	c.Assert(cfg.AllAttrs(), jc.DeepEquals, expected)

	s.fake.Stub.CheckCallNames(c,
		"PrepareConfig",
		"Validate",
	)
	validateCall := s.fake.Stub.Calls()[1]
	c.Assert(validateCall.Args, gc.HasLen, 2)
	c.Assert(validateCall.Args[0], gc.Equals, cfg)
	c.Assert(validateCall.Args[1], gc.IsNil)
}

func (s *ModelConfigCreatorSuite) TestCreateModelCheckAuthorizedKeys(c *gc.C) {
	authorizedKeys := `
			ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDLNN6YxkRJ8liYGh9qymZi23lDRlFrD3ujGfcgkjqa7vOqBHJaWklaIW4vFX0XkYuhgnDlXREi7RRK+4I0XBD051LxADobguLXyeGoOhSRlLLThYMF7Ui8nNylLxY0MYpKUIE6ejve2DHtrwGXBJBUXGJr8z5gKuIZD9J39B3ld1e7v2fpK3SqQ84H8mSZxPBbZqA0NIoq9wl+ke780fYsDxBpsAJhaZW2SjCqcrmNc3m9HgYwzeHhsXDZN2xonoyK2UVMGCsqR0vTHZNpnhME4FdGsmK6WIRMq+z5Mxrw3rSYIgbWi1uACfSsPeBMXmkWORujZrf1w1OKoy1dKeWp juju-client-key
ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAACAQCwvae4M/oc+p5d3vj5TBS/4Mx+us5nVuMBgpYCQYq1Bk+QyfyefVfhQwuILAhmzehKnxUse1kGERQ0wNCtn7wU/HhvAuzQBWkxMvShyO9x7GS+4cDEGhkhMGGCu5NvBBCvp24+WdNeqsvoMDRHtBO1kFVc3FQZ01IjR+FTAICW5hE8e7ssCFK+pIDa8TI44rz41grytVJ1iACvaXc7nTyFZg95EXxSurPv0EnO82Gxfdt4bkiSXPXQqNcTLNiJ2oKRyDVYAjZNIr2Yf+UGCK9fy0VAdM7dwVZ9FOQX430blrDpDNo096+FXs2MoRB5SLzueZo2Eurya5OxcYpfIkdrzNpgAUgiL7cVURCh0+xJrIX/Ow9Axle+GvDcWAS9aZsRO+nsJ9Mry0zGWN/2IAEEZY9KVr7YO8xcCJ/yZ2gFXhyRAjD2oNBBrIfwpFNHZ35TbT5znmTX1wrJapLPyXqosGHZed8FkTDIyocCZzDlB0PpuBzUtjWp8gKwrPNsBGzTMvso3Qah3xOiznc7DTBCeSf2mqsX+6iY6p2k4YmF9LST+hepbgF4WW8Y3xgSuJ510TE3wtf/QZXDjQY+r7+yLraHSlE6CzQvL07snDyn4NHqfGw3GMAT71dpoa7WVGWW4HdcpCa8ALCtOx1GpyaydFANwNuwr1wOMQuY/9R5dw== juju-system-key
`
	baseConfig, err := config.New(
		config.UseDefaults,
		coretesting.FakeConfig().Merge(coretesting.Attrs{
			"type":            "fake",
			"restricted":      "area51",
			"agent-version":   "2.0.0",
			"authorized-keys": authorizedKeys,
		}),
	)
	c.Check(err, jc.ErrorIsNil)
	cfg, err := s.newModelConfig(coretesting.Attrs(baseConfig.AllAttrs()))
	c.Check(err, jc.ErrorIsNil)
	c.Assert(cfg.AuthorizedKeys(), jc.DeepEquals, authorizedKeys)
}

func (s *ModelConfigCreatorSuite) TestCreateModelSameAgentVersion(c *gc.C) {
	cfg, err := s.newModelConfig(coretesting.Attrs(
		s.baseConfig.AllAttrs(),
	).Merge(coretesting.Attrs{
		"name": "new-model",
		"uuid": uuid.MustNewUUID().String(),
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
		"uuid":          uuid.MustNewUUID().String(),
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
		"uuid":          uuid.MustNewUUID().String(),
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
		"uuid":          uuid.MustNewUUID().String(),
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
		"uuid":          uuid.MustNewUUID().String(),
		"agent-version": "1.9.9",
	}))
	c.Assert(err, gc.ErrorMatches, "no agent binaries found for version .*")
}

type fakeProvider struct {
	testing.Stub
	environs.EnvironProvider
	restrictedConfigAttributes []string
}

func (p *fakeProvider) Validate(cfg, old *config.Config) (*config.Config, error) {
	p.MethodCall(p, "Validate", cfg, old)
	return cfg, p.NextErr()
}

func (p *fakeProvider) PrepareConfig(args environs.PrepareConfigParams) (*config.Config, error) {
	p.MethodCall(p, "PrepareConfig", args)
	return args.Config, p.NextErr()
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

func (p *fakeProvider) DetectCredentials(cloudName string) (*cloud.CloudCredential, error) {
	return nil, errors.NotFoundf("credentials")
}
