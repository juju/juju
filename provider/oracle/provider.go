// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oracle

import (
	"github.com/juju/errors"
	oci "github.com/juju/go-oracle-cloud/api"
	"github.com/juju/jsonschema"
	"github.com/juju/loggo"
	"github.com/juju/schema"
	"github.com/juju/utils/clock"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
)

var logger = loggo.GetLogger("juju.provider.oracle")

const (
	providerType = "oracle"
)

// EnvironProvider type implements environs.EnvironProvider interface
type EnvironProvider struct{}

var cloudSchema = &jsonschema.Schema{
	Type:     []jsonschema.Type{jsonschema.ObjectType},
	Required: []string{cloud.EndpointKey, cloud.AuthTypesKey},
	Order:    []string{cloud.EndpointKey, cloud.AuthTypesKey},
	Properties: map[string]*jsonschema.Schema{
		cloud.EndpointKey: {
			Singular: "the API endpoint url for the cloud",
			Type:     []jsonschema.Type{jsonschema.StringType},
			Format:   jsonschema.FormatURI,
		},
		cloud.AuthTypesKey: {
			// don't need a prompt, since there's only one choice.
			Type: []jsonschema.Type{jsonschema.ArrayType},
			Enum: []interface{}{[]string{string(cloud.UserPassAuthType)}},
		},
	},
}

// CloudSchema is defined on the environs.EnvironProvider interface.
func (e EnvironProvider) CloudSchema() *jsonschema.Schema {
	return cloudSchema
}

// Ping is defined on the environs.EnvironProvider interface.
func (e EnvironProvider) Ping(endpoint string) error {
	return nil
}

// PrepareConfig is defined on the environs.EnvironProvider interface.
func (e EnvironProvider) PrepareConfig(config environs.PrepareConfigParams) (*config.Config, error) {
	if err := e.validateCloudSpec(config.Cloud); err != nil {
		return nil, errors.Annotatef(err, "validating cloud spec")
	}

	return config.Config, nil
}

// validateCloudSpec validates the given configuration against the oracle cloud spec
func (e EnvironProvider) validateCloudSpec(spec environs.CloudSpec) error {
	if err := spec.Validate(); err != nil {
		return errors.Trace(err)
	}
	if spec.Credential == nil {
		return errors.NotValidf("missing credentials")
	}

	// validate the authentication type
	if authType := spec.Credential.AuthType(); authType != cloud.UserPassAuthType {
		return errors.NotSupportedf("%q auth-type ", authType)
	}

	if _, ok := spec.Credential.Attributes()["identity-domain"]; !ok {
		return errors.NotFoundf("identity-domain in the credentials")
	}

	return nil
}

// Version is part of the EnvironProvider interface.
func (EnvironProvider) Version() int {
	return 0
}

// Open is defined on the environs.EnvironProvider interface.
func (e *EnvironProvider) Open(params environs.OpenParams) (environs.Environ, error) {
	logger.Debugf("opening model %q", params.Config.Name())
	if err := e.validateCloudSpec(params.Cloud); err != nil {
		return nil, errors.Annotatef(err, "validating cloud spec")
	}

	cli, err := oci.NewClient(oci.Config{
		Username: params.Cloud.Credential.Attributes()["username"],
		Password: params.Cloud.Credential.Attributes()["password"],
		Endpoint: params.Cloud.Endpoint,
		Identify: params.Cloud.Credential.Attributes()["identity-domain"],
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	if err = cli.Authenticate(); err != nil {
		return nil, errors.Trace(err)
	}

	return NewOracleEnviron(e, params, cli, clock.WallClock)
}

// Validate is defined on the config.Validator interface.
func (e EnvironProvider) Validate(cfg, old *config.Config) (valid *config.Config, err error) {
	if err := config.Validate(cfg, old); err != nil {
		return nil, err
	}
	newAttrs, err := cfg.ValidateUnknownAttrs(
		schema.Fields{}, schema.Defaults{},
	)
	if err != nil {
		return nil, err
	}

	return cfg.Apply(newAttrs)
}

var credentials = map[cloud.AuthType]cloud.CredentialSchema{
	cloud.UserPassAuthType: {{
		"username", cloud.CredentialAttr{
			Description: "account username",
		},
	}, {
		"password", cloud.CredentialAttr{
			Description: "account password",
			Hidden:      true,
		},
	}, {
		"identity-domain", cloud.CredentialAttr{
			Description: "indetity domain of the oracle account",
		},
	}},
}

// CredentialSchemas is defined on the environs.ProviderCredentials interface.
func (e EnvironProvider) CredentialSchemas() map[cloud.AuthType]cloud.CredentialSchema {
	return credentials
}

// DetectCredentials is defined on the environs.ProviderCredentials interface.
func (e EnvironProvider) DetectCredentials() (*cloud.CloudCredential, error) {
	return nil, errors.NotFoundf("credentials")
}

// FinalizeCredential is defined on the environs.ProviderCredentials interface.
func (e EnvironProvider) FinalizeCredential(
	cfx environs.FinalizeCredentialContext,
	params environs.FinalizeCredentialParams,
) (*cloud.Credential, error) {

	return &params.Credential, nil
}
