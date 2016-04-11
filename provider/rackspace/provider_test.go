// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rackspace_test

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/errors"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/provider/rackspace"
	coretesting "github.com/juju/juju/testing"
)

type providerSuite struct {
	provider      environs.EnvironProvider
	innerProvider *fakeProvider
}

var _ = gc.Suite(&providerSuite{})

func (s *providerSuite) SetUpTest(c *gc.C) {
	s.innerProvider = new(fakeProvider)
	s.provider = rackspace.NewProvider(s.innerProvider)
}

func (s *providerSuite) TestValidate(c *gc.C) {
	cfg, err := config.New(config.UseDefaults, map[string]interface{}{
		"name":            "some-name",
		"type":            "some-type",
		"uuid":            coretesting.ModelTag.Id(),
		"controller-uuid": coretesting.ModelTag.Id(),
		"authorized-keys": "key",
	})
	c.Check(err, gc.IsNil)
	_, err = s.provider.Validate(cfg, nil)
	c.Check(err, gc.IsNil)
	s.innerProvider.CheckCallNames(c, "Validate")
}

func (s *providerSuite) TestBootstrapConfig(c *gc.C) {
	args := environs.BootstrapConfigParams{CloudRegion: "dfw"}
	s.provider.BootstrapConfig(args)

	expect := args
	expect.CloudRegion = "DFW"
	s.innerProvider.CheckCalls(c, []testing.StubCall{
		{"BootstrapConfig", []interface{}{expect}},
	})
}

type fakeProvider struct {
	testing.Stub
}

func (p *fakeProvider) Open(cfg *config.Config) (environs.Environ, error) {
	p.MethodCall(p, "Open", cfg)
	return nil, nil
}

func (p *fakeProvider) RestrictedConfigAttributes() []string {
	p.MethodCall(p, "RestrictedConfigAttributes")
	return nil
}

func (p *fakeProvider) PrepareForCreateEnvironment(cfg *config.Config) (*config.Config, error) {
	p.MethodCall(p, "PrepareForCreateEnvironment", cfg)
	return nil, nil
}

func (p *fakeProvider) BootstrapConfig(args environs.BootstrapConfigParams) (*config.Config, error) {
	p.MethodCall(p, "BootstrapConfig", args)
	return nil, nil
}

func (p *fakeProvider) PrepareForBootstrap(ctx environs.BootstrapContext, cfg *config.Config) (environs.Environ, error) {
	p.MethodCall(p, "PrepareForBootstrap", ctx, cfg)
	return nil, nil
}

func (p *fakeProvider) Validate(cfg, old *config.Config) (valid *config.Config, err error) {
	p.MethodCall(p, "Validate", cfg, old)
	return cfg, nil
}

func (p *fakeProvider) SecretAttrs(cfg *config.Config) (map[string]string, error) {
	p.MethodCall(p, "SecretAttrs", cfg)
	return nil, nil
}

func (p *fakeProvider) CredentialSchemas() map[cloud.AuthType]cloud.CredentialSchema {
	p.MethodCall(p, "CredentialSchemas")
	return nil
}

func (p *fakeProvider) DetectCredentials() (*cloud.CloudCredential, error) {
	p.MethodCall(p, "DetectCredentials")
	return nil, errors.NotFoundf("credentials")
}
