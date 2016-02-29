// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rackspace_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/errors"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/provider/rackspace"
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
		"authorized-keys": "key",
	})
	c.Check(err, gc.IsNil)
	_, err = s.provider.Validate(cfg, nil)
	c.Check(err, gc.IsNil)
	c.Check(s.innerProvider.Pop().name, gc.Equals, "Validate")
}

type fakeProvider struct {
	methodCalls []methodCall
}

func (p *fakeProvider) Push(name string, params ...interface{}) {
	p.methodCalls = append(p.methodCalls, methodCall{name, params})
}

func (p *fakeProvider) Pop() methodCall {
	m := p.methodCalls[0]
	p.methodCalls = p.methodCalls[1:]
	return m
}

func (p *fakeProvider) Open(cfg *config.Config) (environs.Environ, error) {
	p.Push("Open", cfg)
	return nil, nil
}

func (p *fakeProvider) RestrictedConfigAttributes() []string {
	p.Push("RestrictedConfigAttributes")
	return nil
}

func (p *fakeProvider) PrepareForCreateEnvironment(cfg *config.Config) (*config.Config, error) {
	p.Push("PrepareForCreateEnvironment", cfg)
	return nil, nil
}

func (p *fakeProvider) PrepareForBootstrap(ctx environs.BootstrapContext, args environs.PrepareForBootstrapParams) (environs.Environ, error) {
	p.Push("PrepareForBootstrap", ctx, args)
	return nil, nil
}

func (p *fakeProvider) Validate(cfg, old *config.Config) (valid *config.Config, err error) {
	p.Push("Validate", cfg, old)
	return cfg, nil
}

func (p *fakeProvider) SecretAttrs(cfg *config.Config) (map[string]string, error) {
	p.Push("SecretAttrs", cfg)
	return nil, nil
}

func (p *fakeProvider) CredentialSchemas() map[cloud.AuthType]cloud.CredentialSchema {
	p.Push("CredentialSchemas")
	return nil
}

func (p *fakeProvider) DetectCredentials() (*cloud.CloudCredential, error) {
	p.Push("DetectCredentials")
	return nil, errors.NotFoundf("credentials")
}
