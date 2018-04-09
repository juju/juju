// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci

import (
	"github.com/juju/errors"
	"github.com/juju/jsonschema"
	"github.com/juju/loggo"
	"github.com/juju/schema"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
)

var logger = loggo.GetLogger("juju.provider.oracle")

// EnvironProvider type implements environs.EnvironProvider interface
type EnvironProvider struct{}

var _ config.ConfigSchemaSource = (*EnvironProvider)(nil)
var _ environs.ProviderSchema = (*EnvironProvider)(nil)

// Schema implements environs.ProviderSchema
func (o *EnvironProvider) Schema() environschema.Fields {
	return nil
}

// ConfigSchema implements config.ConfigSchemaSource
func (o *EnvironProvider) ConfigSchema() schema.Fields {
	return nil
}

// ConfigDefaults implements config.ConfigSchemaSource
func (o *EnvironProvider) ConfigDefaults() schema.Defaults {
	return nil
}

// Version implements environs.EnvironProvider.
func (e EnvironProvider) Version() int {
	return 1
}

// CloudSchema implements environs.EnvironProvider.
func (e EnvironProvider) CloudSchema() *jsonschema.Schema {
	return nil
}

// Ping implements environs.EnvironProvider.
func (e *EnvironProvider) Ping(endpoint string) error {
	return errors.NotImplementedf("Ping")
}

// PrepareConfig implements environs.EnvironProvider.
func (e EnvironProvider) PrepareConfig(args environs.PrepareConfigParams) (*config.Config, error) {
	return nil, errors.NotImplementedf("Ping")
}

// Open implements environs.EnvironProvider.
func (e *EnvironProvider) Open(params environs.OpenParams) (environs.Environ, error) {
	return nil, errors.NotImplementedf("Open")
}

// CredentialSchemas implements environs.ProviderCredentials.
func (e EnvironProvider) CredentialSchemas() map[cloud.AuthType]cloud.CredentialSchema {
	return nil
}

// DetectCredentials implements environs.ProviderCredentials.
func (e EnvironProvider) DetectCredentials() (*cloud.CloudCredential, error) {
	return nil, errors.NotImplementedf("DetectCredentials")
}

// FinalizeCredential implements environs.ProviderCredentials.
func (e EnvironProvider) FinalizeCredential(
	ctx environs.FinalizeCredentialContext,
	params environs.FinalizeCredentialParams) (*cloud.Credential, error) {

	return nil, errors.NotImplementedf("FinalizeCredential")
}

// Validate implements config.Validator.
func (e EnvironProvider) Validate(cfg, old *config.Config) (valid *config.Config, err error) {
	return nil, errors.NotImplementedf("Validate")
}
