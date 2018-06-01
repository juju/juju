// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"github.com/juju/errors"
	"github.com/juju/jsonschema"
	"github.com/juju/schema"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
)

const (
	// provider version 1 introduces labels for disks,
	// for associating them with a model and controller.
	providerVersion1 = 1

	currentProviderVersion = providerVersion1
)

type environProvider struct {
	environProviderCredentials
}

var providerInstance environProvider

// Version is part of the EnvironProvider interface.
func (environProvider) Version() int {
	return currentProviderVersion
}

// Open implements environs.EnvironProvider.
func (environProvider) Open(args environs.OpenParams) (environs.Environ, error) {
	if err := validateCloudSpec(args.Cloud); err != nil {
		return nil, errors.Annotate(err, "validating cloud spec")
	}
	env, err := newEnviron(args.Cloud, args.Config)
	return env, errors.Trace(err)
}

// CloudSchema returns the schema used to validate input for add-cloud.  Since
// this provider does not support custom clouds, this always returns nil.
func (p environProvider) CloudSchema() *jsonschema.Schema {
	return nil
}

// Ping tests the connection to the cloud, to verify the endpoint is valid.
func (p environProvider) Ping(ctx context.ProviderCallContext, endpoint string) error {
	return errors.NotImplementedf("Ping")
}

// PrepareConfig implements environs.EnvironProvider.
func (p environProvider) PrepareConfig(args environs.PrepareConfigParams) (*config.Config, error) {
	if err := validateCloudSpec(args.Cloud); err != nil {
		return nil, errors.Annotate(err, "validating cloud spec")
	}
	return configWithDefaults(args.Config)
}

func validateCloudSpec(spec environs.CloudSpec) error {
	if err := spec.Validate(); err != nil {
		return errors.Trace(err)
	}
	if spec.Credential == nil {
		return errors.NotValidf("missing credential")
	}
	switch authType := spec.Credential.AuthType(); authType {
	case cloud.OAuth2AuthType, cloud.JSONFileAuthType:
	default:
		return errors.NotSupportedf("%q auth-type", authType)
	}
	return nil
}

// Schema returns the configuration schema for an environment.
func (environProvider) Schema() environschema.Fields {
	fields, err := config.Schema(configSchema)
	if err != nil {
		panic(err)
	}
	return fields
}

// ConfigSchema returns extra config attributes specific
// to this provider only.
func (p environProvider) ConfigSchema() schema.Fields {
	return configFields
}

// ConfigDefaults returns the default values for the
// provider specific config attributes.
func (p environProvider) ConfigDefaults() schema.Defaults {
	return configDefaults
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

// Validate implements environs.EnvironProvider.Validate.
func (environProvider) Validate(cfg, old *config.Config) (*config.Config, error) {
	newCfg, err := newConfig(cfg, old)
	if err != nil {
		return nil, errors.Annotate(err, "invalid config")
	}
	return newCfg.config, nil
}
