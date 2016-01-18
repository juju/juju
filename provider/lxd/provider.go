// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd

import (
	"github.com/juju/errors"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
)

type environProvider struct{}

var providerInstance environProvider

// Open implements environs.EnvironProvider.
func (environProvider) Open(cfg *config.Config) (environs.Environ, error) {
	// TODO(ericsnow) verify prerequisites (see provider/local/prereq.go)?
	// TODO(ericsnow) do something similar to correctLocalhostURLs()
	// (in provider/local/environprovider.go)?

	env, err := newEnviron(cfg, newRawProvider)
	return env, errors.Trace(err)
}

// PrepareForBootstrap implements environs.EnvironProvider.
func (p environProvider) PrepareForBootstrap(ctx environs.BootstrapContext, cfg *config.Config) (environs.Environ, error) {
	cfg, err := p.PrepareForCreateEnvironment(cfg)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// TODO(ericsnow) Do some of what happens in local provider's
	// PrepareForBootstrap()? Only if "remote" is local host?

	env, err := newEnviron(cfg, newRawProvider)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if ctx.ShouldVerifyCredentials() {
		if err := env.verifyCredentials(); err != nil {
			return nil, errors.Trace(err)
		}
	}
	return env, nil
}

// PrepareForCreateEnvironment is specified in the EnvironProvider interface.
func (environProvider) PrepareForCreateEnvironment(cfg *config.Config) (*config.Config, error) {
	return cfg, nil
}

// RestrictedConfigAttributes is specified in the EnvironProvider interface.
func (environProvider) RestrictedConfigAttributes() []string {
	return []string{
		"remote-url",
		"client-cert",
		"client-key",
	}
}

// Validate implements environs.EnvironProvider.
func (environProvider) Validate(cfg, old *config.Config) (valid *config.Config, err error) {
	if old == nil {
		ecfg, err := newValidConfig(cfg, configDefaults)
		if err != nil {
			return nil, errors.Annotate(err, "invalid config")
		}
		return ecfg.Config, nil
	}

	// The defaults should be set already, so we pass nil.
	ecfg, err := newValidConfig(old, nil)
	if err != nil {
		return nil, errors.Annotate(err, "invalid base config")
	}

	if err := ecfg.update(cfg); err != nil {
		return nil, errors.Annotate(err, "invalid config change")
	}

	return ecfg.Config, nil
}

// SecretAttrs implements environs.EnvironProvider.
func (environProvider) SecretAttrs(cfg *config.Config) (map[string]string, error) {
	// The defaults should be set already, so we pass nil.
	ecfg, err := newValidConfig(cfg, nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return ecfg.secret(), nil
}

// BoilerplateConfig implements environs.EnvironProvider.
func (environProvider) BoilerplateConfig() string {
	// boilerplateConfig is kept in config.go, in the hope that people editing
	// config will keep it up to date.
	return boilerplateConfig
}
