// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"github.com/juju/errors"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
)

type environProvider struct{}

var providerInstance environProvider

// Open implements environs.EnvironProvider.
func (environProvider) Open(cfg *config.Config) (environs.Environ, error) {
	// The config will have come from either state or from a config
	// file. In either case, the original config came from the env from
	// a previous call to the Prepare method. That means there is no
	// need to update the config, e.g. with defaults and OS env values
	// before we validate it, so we pass nil.
	ecfg, err := newValidConfig(cfg, nil)
	if err != nil {
		return nil, errors.Annotate(err, "invalid config")
	}

	env, err := newEnviron(ecfg)
	return env, errors.Trace(err)
}

// PrepareForBootstrap implements environs.EnvironProvider.
func (p environProvider) PrepareForBootstrap(ctx environs.BootstrapContext, cfg *config.Config) (environs.Environ, error) {
	// The config generate here will be store in a config file and in
	// the state DB. So this is the only place we have to update the
	// config with GCE-specific data, e.g. defaults and OS env values.
	ecfg, err := prepareConfig(cfg)
	if err != nil {
		return nil, errors.Annotate(err, "invalid config")
	}

	env, err := newEnviron(ecfg)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if ctx.ShouldVerifyCredentials() {
		if err := env.gce.VerifyCredentials(); err != nil {
			return nil, err
		}
	}
	return env, nil
}

// PrepareForCreateEnvironment is specified in the EnvironProvider interface.
func (environProvider) PrepareForCreateEnvironment(cfg *config.Config) (*config.Config, error) {
	return nil, errors.NotImplementedf("PrepareForCreateEnvironment")
}

// RestrictedConfigAttributes is specified in the EnvironProvider interface.
func (environProvider) RestrictedConfigAttributes() []string {
	return []string{
		cfgPrivateKey,
		cfgClientID,
		cfgClientEmail,
		cfgRegion,
		cfgProjectID,
		cfgImageEndpoint,
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
