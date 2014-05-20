// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
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

// To test UpdateEnvironConfig updates state, Validate returns a config
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
	s.policy.getConfigValidator = func(string) (state.ConfigValidator, error) {
		return &s.configValidator, nil
	}
}

func (s *ConfigValidatorSuite) updateEnvironConfig(c *gc.C) error {
	updateAttrs := map[string]interface{}{
		"authorized-keys": "different-keys",
		"arbitrary-key":   "shazam!",
	}
	return s.State.UpdateEnvironConfig(updateAttrs, nil, nil)
}

func (s *ConfigValidatorSuite) TestConfigValidate(c *gc.C) {
	err := s.updateEnvironConfig(c)
	c.Assert(err, gc.IsNil)
}

func (s *ConfigValidatorSuite) TestUpdateEnvironConfigFailsOnConfigValidateError(c *gc.C) {
	var configValidatorErr error
	s.policy.getConfigValidator = func(string) (state.ConfigValidator, error) {
		configValidatorErr = errors.NotFoundf("")
		return &s.configValidator, configValidatorErr
	}

	err := s.updateEnvironConfig(c)
	c.Assert(err, gc.ErrorMatches, " not found")
}

func (s *ConfigValidatorSuite) TestUpdateEnvironConfigUpdatesState(c *gc.C) {
	s.updateEnvironConfig(c)
	stateCfg, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	newValidCfg, err := mockValidCfg()
	c.Assert(err, gc.IsNil)
	c.Assert(stateCfg.AllAttrs()["arbitrary-key"], gc.Equals, newValidCfg.AllAttrs()["arbitrary-key"])
}

func (s *ConfigValidatorSuite) TestConfigValidateUnimplemented(c *gc.C) {
	var configValidatorErr error
	s.policy.getConfigValidator = func(string) (state.ConfigValidator, error) {
		return nil, configValidatorErr
	}

	err := s.updateEnvironConfig(c)
	c.Assert(err, gc.ErrorMatches, "policy returned nil configValidator without an error")
	configValidatorErr = errors.NotImplementedf("Validator")
	err = s.updateEnvironConfig(c)
	c.Assert(err, gc.IsNil)
}

func (s *ConfigValidatorSuite) TestConfigValidateNoPolicy(c *gc.C) {
	s.policy.getConfigValidator = func(providerType string) (state.ConfigValidator, error) {
		c.Errorf("should not have been invoked")
		return nil, nil
	}

	state.SetPolicy(s.State, nil)
	err := s.updateEnvironConfig(c)
	c.Assert(err, gc.IsNil)
}
