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
	environs.EnvironProvider
}

var providerInstance environProvider

func (p environProvider) setConfigurator(env environs.Environ, err error) (environs.Environ, error) {
	if err != nil {
		return nil, err
	}
	if osEnviron, ok := env.(*openstack.Environ); ok {
		osEnviron.SetProviderConfigurator(new(rackspaceProviderConfigurator))
		return environ{env}, err
	}
	return nil, errors.Errorf("Expected openstack.Environ, but got: %T", env)
}

// Open implements environs.EnvironProvider.
func (p environProvider) Open(cfg *config.Config) (environs.Environ, error) {
	env, err := p.EnvironProvider.Open(cfg)
	res, err := p.setConfigurator(env, err)
	return res, errors.Trace(err)
}

// PrepareForBootstrap implements environs.EnvironProvider.
func (p environProvider) PrepareForBootstrap(ctx environs.BootstrapContext, cfg *config.Config) (environs.Environ, error) {
	env, err := p.EnvironProvider.PrepareForBootstrap(ctx, cfg)
	res, err := p.setConfigurator(env, err)
	return res, errors.Trace(err)
}

// Validate implements environs.EnvironProvider.
func (p environProvider) Validate(cfg, old *config.Config) (valid *config.Config, err error) {
<<<<<<< HEAD
	return p.openstackProvider.Validate(cfg, old)
=======
	cfg, err = cfg.Apply(map[string]interface{}{
		"use-floating-ip":      false,
		"use-default-secgroup": false,
		"auth-url":             "https://identity.api.rackspacecloud.com/v2.0",
	})
	if err != nil {
		return nil, err
	}
	return p.EnvironProvider.Validate(cfg, old)
>>>>>>> review comments implemented
}

// BoilerplateConfig implements environs.EnvironProvider.
func (p environProvider) BoilerplateConfig() string {
	return p.openstackProvider.BoilerplateConfig()
}
