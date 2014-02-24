// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	gc "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/errors"
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

// To test SetEnvironConfig updates state, Validate returns a config
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
	s.policy.getConfigValidator = func(*config.Config) (state.ConfigValidator, error) {
		return &s.configValidator, nil
	}
}

func (s *ConfigValidatorSuite) setEnvironConfig(c *gc.C) error {
	cfg, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	change, err := cfg.Apply(map[string]interface{}{
		"authorized-keys": "different-keys",
		"arbitrary-key":   "shazam!",
	})
	c.Assert(err, gc.IsNil)
	return s.State.SetEnvironConfig(change, cfg)
}

func (s *ConfigValidatorSuite) TestConfigValidate(c *gc.C) {
	err := s.setEnvironConfig(c)
	c.Assert(err, gc.IsNil)
}

func (s *ConfigValidatorSuite) TestSetEnvironConfigFailsOnConfigValidateError(c *gc.C) {
	var configValidatorErr error
	s.policy.getConfigValidator = func(*config.Config) (state.ConfigValidator, error) {
		configValidatorErr = errors.NotFoundf("")
		return &s.configValidator, configValidatorErr
	}

	err := s.setEnvironConfig(c)
	c.Assert(err, gc.ErrorMatches, " not found")
}

func (s *ConfigValidatorSuite) TestSetEnvironConfigUpdatesState(c *gc.C) {
	s.setEnvironConfig(c)
	stateCfg, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	newValidCfg, err := mockValidCfg()
	c.Assert(err, gc.IsNil)
	c.Assert(stateCfg, gc.DeepEquals, newValidCfg)
}

func (s *ConfigValidatorSuite) TestConfigValidateUnimplemented(c *gc.C) {
	var configValidatorErr error
	s.policy.getConfigValidator = func(*config.Config) (state.ConfigValidator, error) {
		return nil, configValidatorErr
	}

	err := s.setEnvironConfig(c)
	c.Assert(err, gc.ErrorMatches, "policy returned nil configValidator without an error")
	configValidatorErr = errors.NewNotImplementedError("Validator")
	err = s.setEnvironConfig(c)
	c.Assert(err, gc.IsNil)
}

func (s *ConfigValidatorSuite) TestConfigValidateNoPolicy(c *gc.C) {
	s.policy.getConfigValidator = func(cfg *config.Config) (state.ConfigValidator, error) {
		c.Errorf("should not have been invoked")
		return nil, nil
	}

	state.SetPolicy(s.State, nil)
	err := s.setEnvironConfig(c)
	c.Assert(err, gc.IsNil)
}
