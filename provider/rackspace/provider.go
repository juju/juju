// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rackspace

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/provider/openstack"
)

var logger = loggo.GetLogger("juju.provider.rackspace")

type environProvider struct {
	openstackProvider environs.EnvironProvider
}

var providerInstance environProvider

// Open implements environs.EnvironProvider.
func (p environProvider) Open(cfg *config.Config) (environs.Environ, error) {
	env, err := p.openstackProvider.Open(cfg)
	if os, ok := env.(*openstack.Environ); ok {
		os.SetProviderConfigurator(new(rackspaceProviderConfigurator))
		return environ{env}, errors.Trace(err)
	}
	return nil, errors.Errorf("Expected openstack.Environ, but got: %T", env)
}

// RestrictedConfigAttributes implements environs.EnvironProvider.
func (p environProvider) RestrictedConfigAttributes() []string {
	return p.openstackProvider.RestrictedConfigAttributes()
}

// PrepareForCreateEnvironment implements environs.EnvironProvider.
func (p environProvider) PrepareForCreateEnvironment(cfg *config.Config) (*config.Config, error) {
	return p.openstackProvider.PrepareForCreateEnvironment(cfg)
}

// PrepareForBootstrap implements environs.EnvironProvider.
func (p environProvider) PrepareForBootstrap(ctx environs.BootstrapContext, cfg *config.Config) (environs.Environ, error) {
	return p.openstackProvider.PrepareForBootstrap(ctx, cfg)
}

// Validate implements environs.EnvironProvider.
func (p environProvider) Validate(cfg, old *config.Config) (valid *config.Config, err error) {
	return p.openstackProvider.Validate(cfg, old)
}

// BoilerplateConfig implements environs.EnvironProvider.
func (p environProvider) BoilerplateConfig() string {
	return p.openstackProvider.BoilerplateConfig()
}

// SecretAttrs implements environs.EnvironProvider.
func (p environProvider) SecretAttrs(cfg *config.Config) (map[string]string, error) {
	return p.openstackProvider.SecretAttrs(cfg)
}
