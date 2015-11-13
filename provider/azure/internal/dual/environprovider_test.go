// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dual_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/provider/azure/internal/dual"
	coretesting "github.com/juju/juju/testing"
)

type environProviderSuite struct {
	coretesting.BaseSuite

	p1, p2        mockEnvironProvider
	dual          *dual.EnvironProvider
	cfg           *config.Config
	isPrimaryFunc func(*config.Config) bool
}

var _ = gc.Suite(&environProviderSuite{})

func (s *environProviderSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.p1 = mockEnvironProvider{}
	s.p2 = mockEnvironProvider{}
	s.dual = dual.NewEnvironProvider(&s.p1, &s.p2, s.isPrimary)
	s.cfg = coretesting.EnvironConfig(c)
	s.isPrimaryFunc = func(*config.Config) bool {
		return true
	}
}

func (s *environProviderSuite) isPrimary(cfg *config.Config) bool {
	return s.isPrimaryFunc(cfg)
}

func (s *environProviderSuite) TestNewEnvironProvider(c *gc.C) {
	// Initially the "active" provider is the primary one, but it will
	// still be overridden when configuration is set as can be seen
	// in subsequent tests.
	c.Assert(s.dual.Active(), gc.Equals, &s.p1)
}

func (s *environProviderSuite) TestIsPrimary(c *gc.C) {
	var calls int
	s.isPrimaryFunc = func(*config.Config) bool {
		calls++
		return false
	}
	for i := 0; i < 2; i++ {
		_, err := s.dual.PrepareForCreateEnvironment(s.cfg)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(calls, gc.Equals, 1) // isPrimary sticks after the first call
	}
}

func (s *environProviderSuite) TestPrepareForCreateEnvironment(c *gc.C) {
	s.testPrepareForCreateEnvironment(c, &s.p1)
}

func (s *environProviderSuite) TestPrepareForCreateEnvironmentIsNotPrimary(c *gc.C) {
	s.isPrimaryFunc = func(*config.Config) bool { return false }
	s.testPrepareForCreateEnvironment(c, &s.p2)
}

func (s *environProviderSuite) testPrepareForCreateEnvironment(c *gc.C, active *mockEnvironProvider) {
	_, err := s.dual.PrepareForCreateEnvironment(s.cfg)
	c.Assert(err, jc.ErrorIsNil)
	s.checkActiveCall(c, active, "PrepareForCreateEnvironment", s.cfg)
}

func (s *environProviderSuite) TestPrepareForBootstrap(c *gc.C) {
	s.testPrepareForBootstrap(c, &s.p1)
}

func (s *environProviderSuite) TestPrepareForBootstrapIsNotPrimary(c *gc.C) {
	s.isPrimaryFunc = func(*config.Config) bool { return false }
	s.testPrepareForBootstrap(c, &s.p2)
}

func (s *environProviderSuite) testPrepareForBootstrap(c *gc.C, active *mockEnvironProvider) {
	ctx := envtesting.BootstrapContext(c)
	_, err := s.dual.PrepareForBootstrap(ctx, s.cfg)
	c.Assert(err, jc.ErrorIsNil)
	s.checkActiveCall(c, active, "PrepareForBootstrap", ctx, s.cfg)
}

func (s *environProviderSuite) TestOpen(c *gc.C) {
	s.testOpen(c, &s.p1)
}

func (s *environProviderSuite) TestOpenIsNotPrimary(c *gc.C) {
	s.isPrimaryFunc = func(*config.Config) bool { return false }
	s.testOpen(c, &s.p2)
}

func (s *environProviderSuite) testOpen(c *gc.C, active *mockEnvironProvider) {
	_, err := s.dual.Open(s.cfg)
	c.Assert(err, jc.ErrorIsNil)
	s.checkActiveCall(c, active, "Open", s.cfg)
}

func (s *environProviderSuite) TestValidate(c *gc.C) {
	s.testValidate(c, &s.p1)
}

func (s *environProviderSuite) TestValidateIsNotPrimary(c *gc.C) {
	s.isPrimaryFunc = func(*config.Config) bool { return false }
	s.testValidate(c, &s.p2)
}

func (s *environProviderSuite) testValidate(c *gc.C, active *mockEnvironProvider) {
	_, err := s.dual.Validate(s.cfg, s.cfg)
	c.Assert(err, jc.ErrorIsNil)
	s.checkActiveCall(c, active, "Validate", s.cfg, s.cfg)
}

func (s *environProviderSuite) TestValidateIsPrimaryMismatch(c *gc.C) {
	cfg2 := coretesting.EnvironConfig(c)
	s.isPrimaryFunc = func(cfg *config.Config) bool {
		return cfg == s.cfg
	}
	_, err := s.dual.Validate(s.cfg, cfg2)
	c.Assert(err, gc.ErrorMatches, "mixing primary and secondary configurations not valid")
	s.isPrimaryFunc = func(cfg *config.Config) bool {
		return cfg == cfg2
	}
	_, err = s.dual.Validate(s.cfg, cfg2)
	c.Assert(err, gc.ErrorMatches, "mixing primary and secondary configurations not valid")
	s.p1.CheckCalls(c, nil) // no calls
	s.p2.CheckCalls(c, nil) // no calls
}

func (s *environProviderSuite) TestSecretAttrs(c *gc.C) {
	s.testSecretAttrs(c, &s.p1)
}

func (s *environProviderSuite) TestSecretAttrsIsNotPrimary(c *gc.C) {
	s.isPrimaryFunc = func(*config.Config) bool { return false }
	s.testSecretAttrs(c, &s.p2)
}

func (s *environProviderSuite) testSecretAttrs(c *gc.C, active *mockEnvironProvider) {
	_, err := s.dual.SecretAttrs(s.cfg)
	c.Assert(err, jc.ErrorIsNil)
	s.checkActiveCall(c, active, "SecretAttrs", s.cfg)
}

func (s *environProviderSuite) TestBoilerplateConfig(c *gc.C) {
	var calls int
	s.isPrimaryFunc = func(*config.Config) bool {
		calls++
		return true
	}
	_ = s.dual.BoilerplateConfig()
	c.Assert(calls, gc.Equals, 0)
	s.checkActiveCall(c, &s.p1, "BoilerplateConfig")
}

func (s *environProviderSuite) TestBoilerplateConfigSecondaryActive(c *gc.C) {
	s.TestOpenIsNotPrimary(c)
	s.p2.ResetCalls()
	_ = s.dual.BoilerplateConfig()
	s.checkActiveCall(c, &s.p2, "BoilerplateConfig")
}

func (s *environProviderSuite) TestRestrictedConfigAttributes(c *gc.C) {
	var calls int
	s.isPrimaryFunc = func(*config.Config) bool {
		calls++
		return true
	}
	_ = s.dual.RestrictedConfigAttributes()
	c.Assert(calls, gc.Equals, 0)
	s.checkActiveCall(c, &s.p1, "RestrictedConfigAttributes")
}

func (s *environProviderSuite) TestRestrictedConfigAttributesSecondaryActive(c *gc.C) {
	s.TestOpenIsNotPrimary(c)
	s.p2.ResetCalls()
	_ = s.dual.RestrictedConfigAttributes()
	s.checkActiveCall(c, &s.p2, "RestrictedConfigAttributes")
}

func (s *environProviderSuite) checkActiveCall(c *gc.C, active *mockEnvironProvider, name string, args ...interface{}) {
	inactive := &s.p1
	if active == inactive {
		inactive = &s.p2
	}
	c.Assert(s.dual.Active(), gc.Equals, active)
	active.CheckCallNames(c, name)
	active.CheckCall(c, 0, name, args...)
	inactive.CheckCallNames(c) // none
}

type mockEnvironProvider struct {
	testing.Stub
}

func (p *mockEnvironProvider) PrepareForCreateEnvironment(cfg *config.Config) (*config.Config, error) {
	p.MethodCall(p, "PrepareForCreateEnvironment", cfg)
	return nil, p.NextErr()
}

func (p *mockEnvironProvider) PrepareForBootstrap(ctx environs.BootstrapContext, cfg *config.Config) (environs.Environ, error) {
	p.MethodCall(p, "PrepareForBootstrap", ctx, cfg)
	return nil, p.NextErr()
}

func (p *mockEnvironProvider) Open(cfg *config.Config) (environs.Environ, error) {
	p.MethodCall(p, "Open", cfg)
	return nil, p.NextErr()
}

func (p *mockEnvironProvider) Validate(newCfg, oldCfg *config.Config) (*config.Config, error) {
	p.MethodCall(p, "Validate", newCfg, oldCfg)
	return nil, p.NextErr()
}

func (p *mockEnvironProvider) SecretAttrs(cfg *config.Config) (map[string]string, error) {
	p.MethodCall(p, "SecretAttrs", cfg)
	return nil, p.NextErr()
}

func (p *mockEnvironProvider) BoilerplateConfig() string {
	p.MethodCall(p, "BoilerplateConfig")
	return ""
}

func (p *mockEnvironProvider) RestrictedConfigAttributes() []string {
	p.MethodCall(p, "RestrictedConfigAttributes")
	return nil
}
