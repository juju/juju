// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"github.com/juju/errors"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
)

type environProvider struct {
	environProviderCredentials
}

var providerInstance environProvider

// Open implements environs.EnvironProvider.
func (environProvider) Open(cfg *config.Config) (environs.Environ, error) {
	env, err := newEnviron(cfg)
	return env, errors.Trace(err)
}

// PrepareForBootstrap implements environs.EnvironProvider.
func (p environProvider) PrepareForBootstrap(ctx environs.BootstrapContext, args environs.PrepareForBootstrapParams) (environs.Environ, error) {
	// Add credentials to the configuration.
	cfg := args.Config
	switch authType := args.Credentials.AuthType(); authType {
	case cloud.JSONFileAuthType:
		var err error
		filename := args.Credentials.Attributes()["file"]
		args.Credentials, err = parseJSONAuthFile(filename)
		if err != nil {
			return nil, errors.Trace(err)
		}
		fallthrough
	case cloud.OAuth2AuthType:
		credentialAttrs := args.Credentials.Attributes()
		var err error
		cfg, err = args.Config.Apply(map[string]interface{}{
			"project-id":   credentialAttrs["project-id"],
			"client-id":    credentialAttrs["client-id"],
			"client-email": credentialAttrs["client-email"],
			"private-key":  credentialAttrs["private-key"],
		})
		if err != nil {
			return nil, errors.Trace(err)
		}
	default:
		return nil, errors.NotSupportedf("%q auth-type", authType)
	}

	cfg, err := p.PrepareForCreateEnvironment(cfg)
	if err != nil {
		return nil, errors.Trace(err)
	}
	env, err := newEnviron(cfg)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if ctx.ShouldVerifyCredentials() {
		if err := env.gce.VerifyCredentials(); err != nil {
			return nil, errors.Trace(err)
		}
	}
	return env, nil
}

// PrepareForCreateEnvironment is specified in the EnvironProvider interface.
func (p environProvider) PrepareForCreateEnvironment(cfg *config.Config) (*config.Config, error) {
	return configWithDefaults(cfg)
}

// UpgradeModelConfig is specified in the ModelConfigUpgrader interface.
func (environProvider) UpgradeConfig(cfg *config.Config) (*config.Config, error) {
	return configWithDefaults(cfg)
}

func configWithDefaults(cfg *config.Config) (*config.Config, error) {
	defaults := make(map[string]interface{})
	if _, ok := cfg.StorageDefaultBlockSource(); !ok {
		// Set the default block source.
		defaults[config.StorageDefaultBlockSourceKey] = storageProviderType
	}
	if len(defaults) == 0 {
		return cfg, nil
	}
	return cfg.Apply(defaults)
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
