// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type ConfigValidatorSuite struct {
	ConnSuite
	configValidator mockConfigValidator
}

var _ = gc.Suite(&ConfigValidatorSuite{})

type mockConfigValidator struct {
	validateError error
	validateCfg   *config.Config
	validateOld   *config.Config
	validateValid *config.Config
}

// To test UpdateModelConfig updates state, Validate returns a config
// different to both input configs
func mockValidCfg() (valid *config.Config, err error) {
	cfg, err := config.New(config.UseDefaults, coretesting.FakeConfig())
	if err != nil {
		return nil, err
	}
	valid, err = cfg.Apply(map[string]interface{}{
		"arbitrary-key": "cptn-marvel",
	})
	if err != nil {
		return nil, err
	}
	return valid, nil
}

func (p *mockConfigValidator) Validate(cfg, old *config.Config) (valid *config.Config, err error) {
	p.validateCfg = cfg
	p.validateOld = old
	p.validateValid, p.validateError = mockValidCfg()
	return p.validateValid, p.validateError
}

func (s *ConfigValidatorSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.configValidator = mockConfigValidator{}
	s.policy.GetConfigValidator = func(string) (state.ConfigValidator, error) {
		return &s.configValidator, nil
	}
}

func (s *ConfigValidatorSuite) updateModelConfig(c *gc.C) error {
	updateAttrs := map[string]interface{}{
		"authorized-keys": "different-keys",
		"arbitrary-key":   "shazam!",
	}
	return s.State.UpdateModelConfig(updateAttrs, nil, nil)
}

func (s *ConfigValidatorSuite) TestConfigValidate(c *gc.C) {
	err := s.updateModelConfig(c)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ConfigValidatorSuite) TestUpdateModelConfigFailsOnConfigValidateError(c *gc.C) {
	var configValidatorErr error
	s.policy.GetConfigValidator = func(string) (state.ConfigValidator, error) {
		configValidatorErr = errors.NotFoundf("")
		return &s.configValidator, configValidatorErr
	}

	err := s.updateModelConfig(c)
	c.Assert(err, gc.ErrorMatches, " not found")
}

func (s *ConfigValidatorSuite) TestUpdateModelConfigUpdatesState(c *gc.C) {
	s.updateModelConfig(c)
	stateCfg, err := s.State.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	newValidCfg, err := mockValidCfg()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stateCfg.AllAttrs()["arbitrary-key"], gc.Equals, newValidCfg.AllAttrs()["arbitrary-key"])
}

func (s *ConfigValidatorSuite) TestConfigValidateUnimplemented(c *gc.C) {
	var configValidatorErr error
	s.policy.GetConfigValidator = func(string) (state.ConfigValidator, error) {
		return nil, configValidatorErr
	}

	err := s.updateModelConfig(c)
	c.Assert(err, gc.ErrorMatches, "policy returned nil configValidator without an error")
	configValidatorErr = errors.NotImplementedf("Validator")
	err = s.updateModelConfig(c)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ConfigValidatorSuite) TestConfigValidateNoPolicy(c *gc.C) {
	s.policy.GetConfigValidator = func(providerType string) (state.ConfigValidator, error) {
		c.Errorf("should not have been invoked")
		return nil, nil
	}

	state.SetPolicy(s.State, nil)
	err := s.updateModelConfig(c)
	c.Assert(err, jc.ErrorIsNil)
}
