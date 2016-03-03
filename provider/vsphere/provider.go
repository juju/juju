// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !gccgo

package vsphere

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
)

type environProvider struct {
	environProviderCredentials
}

var providerInstance = environProvider{}
var _ environs.EnvironProvider = providerInstance

var logger = loggo.GetLogger("juju.provider.vmware")

// Open implements environs.EnvironProvider.
func (environProvider) Open(cfg *config.Config) (environs.Environ, error) {
	env, err := newEnviron(cfg)
	return env, errors.Trace(err)
}

// PrepareForBootstrap implements environs.EnvironProvider.
func (p environProvider) PrepareForBootstrap(ctx environs.BootstrapContext, args environs.PrepareForBootstrapParams) (environs.Environ, error) {

	cfg := args.Config
	switch authType := args.Credentials.AuthType(); authType {
	case cloud.UserPassAuthType:
		credentialAttrs := args.Credentials.Attributes()
		var err error
		cfg, err = cfg.Apply(map[string]interface{}{
			cfgUser:     credentialAttrs["user"],
			cfgPassword: credentialAttrs["password"],
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

	return env, nil
}

// PrepareForCreateEnvironment is specified in the EnvironProvider interface.
func (environProvider) PrepareForCreateEnvironment(cfg *config.Config) (*config.Config, error) {
	return cfg, nil
}

// RestrictedConfigAttributes is specified in the EnvironProvider interface.
func (environProvider) RestrictedConfigAttributes() []string {
	return []string{
		cfgDatacenter,
		cfgHost,
		cfgUser,
		cfgPassword,
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
