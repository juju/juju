// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/jsonschema"
	"github.com/juju/schema"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/configschema"
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
func (environProvider) Open(ctx context.Context, args environs.OpenParams, invalidator environs.CredentialInvalidator) (environs.Environ, error) {
	if err := validateCloudSpec(args.Cloud); err != nil {
		return nil, errors.Annotate(err, "validating cloud spec")
	}
	env, err := newEnviron(ctx, args.Cloud, args.Config, invalidator)
	return env, errors.Trace(err)
}

// CloudSchema returns the schema used to validate input for add-cloud.  Since
// this provider does not support custom clouds, this always returns nil.
func (p environProvider) CloudSchema() *jsonschema.Schema {
	return nil
}

// Ping tests the connection to the cloud, to verify the endpoint is valid.
func (p environProvider) Ping(_ context.Context, _ string) error {
	return errors.NotImplementedf("Ping")
}

// ModelConfigDefaults provides a set of default model config attributes that
// should be set on a models config if they have not been specified by the user.
func (p environProvider) ModelConfigDefaults(_ context.Context) (map[string]any, error) {
	return map[string]any{
		config.StorageDefaultBlockSourceKey: storageProviderType,
	}, nil
}

// ValidateCloud is specified in the EnvironProvider interface.
func (environProvider) ValidateCloud(ctx context.Context, spec environscloudspec.CloudSpec) error {
	return errors.Annotate(validateCloudSpec(spec), "validating cloud spec")
}

func validateCloudSpec(spec environscloudspec.CloudSpec) error {
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
func (environProvider) Schema() configschema.Fields {
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

// Validate implements environs.EnvironProvider.Validate.
func (environProvider) Validate(ctx context.Context, cfg, old *config.Config) (*config.Config, error) {
	newCfg, err := newConfig(ctx, cfg, old)
	if err != nil {
		return nil, errors.Annotate(err, "invalid config")
	}
	return newCfg.config, nil
}
