// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !gccgo

package vsphere

import (
	"net/url"

	"github.com/juju/errors"
	"github.com/juju/jsonschema"
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
func (environProvider) Open(args environs.OpenParams) (environs.Environ, error) {
	if err := validateCloudSpec(args.Cloud); err != nil {
		return nil, errors.Annotate(err, "validating cloud spec")
	}
	env, err := newEnviron(args.Cloud, args.Config)
	return env, errors.Trace(err)
}

var cloudSchema = &jsonschema.Schema{
	Type:     []jsonschema.Type{jsonschema.ObjectType},
	Required: []string{cloud.EndpointKey, cloud.AuthTypesKey, cloud.RegionsKey},
	Order:    []string{cloud.EndpointKey, cloud.AuthTypesKey, cloud.RegionsKey},
	Properties: map[string]*jsonschema.Schema{
		cloud.EndpointKey: {
			Singular: "the API endpoint url for the cloud",
			Type:     []jsonschema.Type{jsonschema.StringType},
			Format:   jsonschema.FormatURI,
		},
		cloud.AuthTypesKey: &jsonschema.Schema{
			// don't need a prompt, since there's only one choice.
			Type: []jsonschema.Type{jsonschema.ArrayType},
			Enum: []interface{}{[]string{string(cloud.UserPassAuthType)}},
		},
		cloud.RegionsKey: {
			Type:     []jsonschema.Type{jsonschema.ObjectType},
			Singular: "region",
			Plural:   "regions",
			AdditionalProperties: &jsonschema.Schema{
				Type:          []jsonschema.Type{jsonschema.ObjectType},
				MaxProperties: jsonschema.Int(0),
			},
		},
	},
}

// CloudSchema returns the schema for adding new clouds of this type.
func (p environProvider) CloudSchema() *jsonschema.Schema {
	return cloudSchema
}

// Ping tests the connection to the cloud, to verify the endpoint is valid.
func (p environProvider) Ping(endpoint string) error {
	// a stub for now until we get a real implementation.
	_, err := url.Parse(endpoint)
	return errors.Trace(err)
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

func validateCloudSpec(spec environs.CloudSpec) error {
	if err := spec.Validate(); err != nil {
		return errors.Trace(err)
	}
	// TODO(axw) add validation of endpoint/region.
	if spec.Credential == nil {
		return errors.NotValidf("missing credential")
	}
	if authType := spec.Credential.AuthType(); authType != cloud.UserPassAuthType {
		return errors.NotSupportedf("%q auth-type", authType)
	}
	return nil
}
