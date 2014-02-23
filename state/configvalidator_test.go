// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	gc "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs"
	coretesting "launchpad.net/juju-core/testing"

	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/state"
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

func (p *mockConfigValidator) Validate(cfg, old *config.Config) (valid *config.Config, err error) {
	p.validateCfg = cfg
	p.validateOld = old
	return p.validateValid, p.validateError
}

func (s *ConfigValidatorSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.configValidator = mockConfigValidator{}
	s.policy.getConfigValidator = func(*config.Config) (state.ConfigValidator, error) {
		return &s.configValidator, nil
	}
}

func getConfig(c *gc.C) *config.Config {
	testConfig, err := config.New(config.UseDefaults, coretesting.FakeConfig())
	c.Assert(err, gc.IsNil)
	return testConfig
}

func getProvider(c *gc.C, cfg *config.Config) environs.EnvironProvider {
	//Get provider type from config
	providerType := cfg.AllAttrs()["type"].(string)
	//Get provider
	provider, err := environs.Provider(providerType)
	c.Assert(err, gc.IsNil)
	return provider
}

func (s *ConfigValidatorSuite) validate(c *gc.C) (valid *config.Config, err error) {
	testConfig := getConfig(c)
	provider := getProvider(c, testConfig)
	return provider.Validate(testConfig, nil)
}

func (s *ConfigValidatorSuite) TestConfigValidate(c *gc.C) {
	_, err := s.validate(c)
	c.Assert(err, gc.IsNil)
}

func (s *ConfigValidatorSuite) TestConfigValidateUnimplemented(c *gc.C) {
	var configValidatorErr error
	s.policy.getConfigValidator = func(*config.Config) (state.ConfigValidator, error) {
		return nil, configValidatorErr
	}

	_, err := s.validate(c)
	c.Assert(err, gc.ErrorMatches, "cannot validate config: policy returned nil validator without an error")
	configValidatorErr = errors.NewNotImplementedError("Validator")
	_, err = s.validate(c)
	c.Assert(err, gc.IsNil)
}

func (s *ConfigValidatorSuite) TestConfigValidateNoPolicy(c *gc.C) {
	s.policy.getConfigValidator = func(*config.Config) (state.ConfigValidator, error) {
		c.Errorf("should not have been invoked")
		return nil, nil
	}

	state.SetPolicy(s.State, nil)
	_, err := s.validate(c)
	c.Assert(err, gc.IsNil)
}
