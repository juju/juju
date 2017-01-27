// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd

import (
	"github.com/juju/errors"
	"github.com/juju/jsonschema"
	"github.com/juju/schema"
	"github.com/juju/utils"
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
	local, err := validateCloudSpec(args.Cloud)
	if err != nil {
		return nil, errors.Annotate(err, "validating cloud spec")
	}
	env, err := newEnviron(
		local,
		args.Cloud,
		args.Config,
		newRawProvider,
	)
	return env, errors.Trace(err)
}

// CloudSchema returns the schema used to validate input for add-cloud.  Since
// this provider does not support custom clouds, this always returns nil.
func (p environProvider) CloudSchema() *jsonschema.Schema {
	return nil
}

// Ping tests the connection to the cloud, to verify the endpoint is valid.
func (p environProvider) Ping(endpoint string) error {
	return errors.NotImplementedf("Ping")
}

// PrepareConfig implements environs.EnvironProvider.
func (p environProvider) PrepareConfig(args environs.PrepareConfigParams) (*config.Config, error) {
	_, err := validateCloudSpec(args.Cloud)
	if err != nil {
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

// DetectClouds implements environs.CloudDetector.
func (environProvider) DetectClouds() ([]cloud.Cloud, error) {
	localhostCloud, err := localhostCloud()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return []cloud.Cloud{localhostCloud}, nil
}

// DetectCloud implements environs.CloudDetector.
func (environProvider) DetectCloud(name string) (cloud.Cloud, error) {
	// For now we just return a hard-coded "localhost" cloud,
	// i.e. the local LXD daemon. We may later want to detect
	// locally-configured remotes.
	switch name {
	case "lxd", "localhost":
		return localhostCloud()
	}
	return cloud.Cloud{}, errors.NotFoundf("cloud %s", name)
}

func localhostCloud() (cloud.Cloud, error) {
	p, err := newLocalRawProvider()
	if err != nil {
		return cloud.Cloud{}, errors.Trace(err)
	}
	hostAddress, err := utils.GetAddressForInterface(p.DefaultProfileBridgeName())
	if err != nil {
		return cloud.Cloud{}, errors.Trace(err)
	}
	return cloud.Cloud{
		Name: lxdnames.DefaultCloud,
		Type: lxdnames.ProviderType,
		AuthTypes: []cloud.AuthType{
			cloud.CertificateAuthType,
		},
		Regions: []cloud.Region{{
			Name:     lxdnames.DefaultRegion,
			Endpoint: hostAddress,
		}},
		Description: cloud.DefaultCloudDescription(lxdnames.ProviderType),
	}, nil
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

func validateCloudSpec(spec environs.CloudSpec) (local bool, _ error) {
	if err := spec.Validate(); err != nil {
		return false, errors.Trace(err)
	}
	if spec.Credential == nil {
		return false, errors.NotValidf("missing credential")
	}
	if spec.Endpoint != "" {
		var err error
		local, err = isLocalEndpoint(spec.Endpoint)
		if err != nil {
			return false, errors.Trace(err)
		}
	} else {
		// No endpoint specified, so assume we're local. This
		// will happen, for example, when destroying a 2.0
		// LXD controller.
		local = true
	}
	switch authType := spec.Credential.AuthType(); authType {
	case cloud.EmptyAuthType:
		if !local {
			// The empty auth-type is only valid
			// when the endpoint is local to the
			// machine running this process.
			return false, errors.NotSupportedf("%q auth-type for non-local LXD", authType)
		}
	case cloud.CertificateAuthType:
		if _, _, ok := getCerts(spec); !ok {
			return false, errors.NotValidf("certificate credentials")
		}
	default:
		return false, errors.NotSupportedf("%q auth-type", authType)
	}
	return local, nil
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
