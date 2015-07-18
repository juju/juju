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
<<<<<<< HEAD
	environs.EnvironProvider
=======
	openstackProvider environs.EnvironProvider
>>>>>>> modifications to opestack provider applied
}

var providerInstance environProvider

<<<<<<< HEAD
func (p environProvider) setConfigurator(env environs.Environ, err error) (environs.Environ, error) {
	if err != nil {
		return nil, errors.Trace(err)
	}
	if osEnviron, ok := env.(*openstack.Environ); ok {
		osEnviron.SetProviderConfigurator(new(rackspaceProviderConfigurator))
=======
// Open implements environs.EnvironProvider.
func (p environProvider) Open(cfg *config.Config) (environs.Environ, error) {
	env, err := p.openstackProvider.Open(cfg)
	if os, ok := env.(*openstack.Environ); ok {
		os.SetProviderConfigurator(new(rackspaceProviderConfigurator))
>>>>>>> modifications to opestack provider applied
		return environ{env}, errors.Trace(err)
	}
	return nil, errors.Errorf("Expected openstack.Environ, but got: %T", env)
}

<<<<<<< HEAD
// Open implements environs.EnvironProvider.
func (p environProvider) Open(cfg *config.Config) (environs.Environ, error) {
	env, err := p.EnvironProvider.Open(cfg)
	res, err := p.setConfigurator(env, err)
	return res, errors.Trace(err)
=======
// RestrictedConfigAttributes implements environs.EnvironProvider.
func (p environProvider) RestrictedConfigAttributes() []string {
	return p.openstackProvider.RestrictedConfigAttributes()
}

// PrepareForCreateEnvironment implements environs.EnvironProvider.
func (p environProvider) PrepareForCreateEnvironment(cfg *config.Config) (*config.Config, error) {
	return p.openstackProvider.PrepareForCreateEnvironment(cfg)
>>>>>>> modifications to opestack provider applied
}

// PrepareForBootstrap implements environs.EnvironProvider.
func (p environProvider) PrepareForBootstrap(ctx environs.BootstrapContext, cfg *config.Config) (environs.Environ, error) {
<<<<<<< HEAD
	env, err := p.EnvironProvider.PrepareForBootstrap(ctx, cfg)
	res, err := p.setConfigurator(env, err)
	return res, errors.Trace(err)
=======
	return p.openstackProvider.PrepareForBootstrap(ctx, cfg)
>>>>>>> modifications to opestack provider applied
}

// Validate implements environs.EnvironProvider.
func (p environProvider) Validate(cfg, old *config.Config) (valid *config.Config, err error) {
<<<<<<< HEAD
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
=======
	return p.openstackProvider.Validate(cfg, old)
>>>>>>> modifications to opestack provider applied
}

// BoilerplateConfig implements environs.EnvironProvider.
func (p environProvider) BoilerplateConfig() string {
	return p.openstackProvider.BoilerplateConfig()
}
<<<<<<< HEAD
=======

// SecretAttrs implements environs.EnvironProvider.
func (p environProvider) SecretAttrs(cfg *config.Config) (map[string]string, error) {
	return p.openstackProvider.SecretAttrs(cfg)
}
>>>>>>> modifications to opestack provider applied
