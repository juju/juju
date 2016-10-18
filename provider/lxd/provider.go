// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/provider/lxd/lxdnames"
)

type environProvider struct {
	environProviderCredentials
}

var providerInstance environProvider

// Open implements environs.EnvironProvider.
func (environProvider) Open(args environs.OpenParams) (environs.Environ, error) {
	if err := validateCloudSpec(args.Cloud); err != nil {
		return nil, errors.Annotate(err, "validating cloud spec")
	}
	// TODO(ericsnow) verify prerequisites (see provider/local/prereq.go)?
	env, err := newEnviron(args.Cloud, args.Config, newRawProvider)
	return env, errors.Trace(err)
}

// PrepareConfig implements environs.EnvironProvider.
func (p environProvider) PrepareConfig(args environs.PrepareConfigParams) (*config.Config, error) {
	if err := validateCloudSpec(args.Cloud); err != nil {
		return nil, errors.Annotate(err, "validating cloud spec")
	}
	return args.Config, nil
}

// Validate implements environs.EnvironProvider.
func (environProvider) Validate(cfg, old *config.Config) (valid *config.Config, err error) {
	if _, err := newValidConfig(cfg); err != nil {
		return nil, errors.Annotate(err, "invalid base config")
	}
	return cfg, nil
}

// DetectRegions implements environs.CloudRegionDetector.
func (environProvider) DetectRegions() ([]cloud.Region, error) {
	// For now we just return a hard-coded "localhost" region,
	// i.e. the local LXD daemon. We may later want to detect
	// locally-configured remotes.
	return []cloud.Region{{Name: lxdnames.DefaultRegion}}, nil
}

// Schema returns the configuration schema for an environment.
func (environProvider) Schema() environschema.Fields {
	fields, err := config.Schema(configSchema)
	if err != nil {
		panic(err)
	}
	return fields
}

func validateCloudSpec(spec environs.CloudSpec) error {
	if err := spec.Validate(); err != nil {
		return errors.Trace(err)
	}
	if spec.Endpoint != "" {
		return errors.NotValidf("non-empty endpoint %q", spec.Endpoint)
	}
	if spec.Credential != nil {
		if authType := spec.Credential.AuthType(); authType != cloud.EmptyAuthType {
			return errors.NotSupportedf("%q auth-type", authType)
		}
	}
	return nil
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
