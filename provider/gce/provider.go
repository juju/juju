// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"github.com/juju/errors"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
)

type environProvider struct {
	environProviderCredentials
}

var providerInstance environProvider

// Open implements environs.EnvironProvider.
func (environProvider) Open(args environs.OpenParams) (environs.Environ, error) {
	env, err := newEnviron(args.Config)
	return env, errors.Trace(err)
}

// PrepareConfig implements environs.EnvironProvider.
func (p environProvider) PrepareConfig(args environs.PrepareConfigParams) (*config.Config, error) {
	// Add credentials to the configuration.
	cfg := args.Config
	switch authType := args.Cloud.Credential.AuthType(); authType {
	case cloud.JSONFileAuthType:
		filename := args.Cloud.Credential.Attributes()["file"]
		credential, err := ParseJSONAuthFile(filename)
		if err != nil {
			return nil, errors.Trace(err)
		}
		args.Cloud.Credential = &credential
		fallthrough
	case cloud.OAuth2AuthType:
		credentialAttrs := args.Cloud.Credential.Attributes()
		var err error
		cfg, err = args.Config.Apply(map[string]interface{}{
			cfgProjectID:   credentialAttrs[cfgProjectID],
			cfgClientID:    credentialAttrs[cfgClientID],
			cfgClientEmail: credentialAttrs[cfgClientEmail],
			cfgPrivateKey:  credentialAttrs[cfgPrivateKey],
		})
		if err != nil {
			return nil, errors.Trace(err)
		}
	default:
		return nil, errors.NotSupportedf("%q auth-type", authType)
	}
	// Ensure cloud info is in config.
	var err error
	cfg, err = cfg.Apply(map[string]interface{}{
		cfgRegion: args.Cloud.Region,
		// TODO (anastasiamac 2016-06-09) at some stage will need to
		//  also add endpoint and storage endpoint.
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return configWithDefaults(cfg)
}

// Schema returns the configuration schema for an environment.
func (environProvider) Schema() environschema.Fields {
	fields, err := config.Schema(configSchema)
	if err != nil {
		panic(err)
	}
	return fields
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
		cfgRegion,
		cfgImageEndpoint,
	}
}

// Validate implements environs.EnvironProvider.Validate.
func (environProvider) Validate(cfg, old *config.Config) (*config.Config, error) {
	newCfg, err := newConfig(cfg, old)
	if err != nil {
		return nil, errors.Annotate(err, "invalid config")
	}
	return newCfg.config, nil
}

// SecretAttrs implements environs.EnvironProvider.SecretAttrs.
func (environProvider) SecretAttrs(cfg *config.Config) (map[string]string, error) {
	// The defaults should be set already, so we pass nil.
	ecfg, err := newConfig(cfg, nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return ecfg.secret(), nil
}
