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
		"controller-uuid": coretesting.ControllerTag.Id(),
		"authorized-keys": "key",
	})
	c.Check(err, gc.IsNil)
	_, err = s.provider.Validate(cfg, nil)
	c.Check(err, gc.IsNil)
	s.innerProvider.CheckCallNames(c, "Validate")
}

func (s *providerSuite) TestPrepareConfig(c *gc.C) {
	args := environs.PrepareConfigParams{
		Cloud: environs.CloudSpec{
			Region: "dfw",
		},
	}
	s.provider.PrepareConfig(args)

	expect := args
	expect.Cloud.Region = "DFW"
	s.innerProvider.CheckCalls(c, []testing.StubCall{
		{"PrepareConfig", []interface{}{expect}},
	})
}

type fakeProvider struct {
	testing.Stub
}

func (p *fakeProvider) Open(args environs.OpenParams) (environs.Environ, error) {
	p.MethodCall(p, "Open", args)
	return nil, nil
}

func (p *fakeProvider) PrepareForCreateEnvironment(controllerUUID string, cfg *config.Config) (*config.Config, error) {
	p.MethodCall(p, "PrepareForCreateEnvironment", controllerUUID, cfg)
	return nil, nil
}

func (p *fakeProvider) PrepareConfig(args environs.PrepareConfigParams) (*config.Config, error) {
	p.MethodCall(p, "PrepareConfig", args)
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

func (p *fakeProvider) CredentialSchemas() map[cloud.AuthType]cloud.CredentialSchema {
	p.MethodCall(p, "CredentialSchemas")
	return nil
}

func (p *fakeProvider) DetectCredentials() (*cloud.CloudCredential, error) {
	p.MethodCall(p, "DetectCredentials")
	return nil, errors.NotFoundf("credentials")
}
