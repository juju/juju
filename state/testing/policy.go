// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	stdcontext "context"

	"github.com/juju/errors"
	"github.com/juju/schema"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/domain/credential"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/internal/storage"
)

type MockPolicy struct {
	GetConfigValidator         func() (config.Validator, error)
	GetConstraintsValidator    func() (constraints.Validator, error)
	GetStorageProviderRegistry func() (storage.ProviderRegistry, error)
}

func (p *MockPolicy) ConfigValidator() (config.Validator, error) {
	if p.GetConfigValidator != nil {
		return p.GetConfigValidator()
	}
	return nil, errors.NotImplementedf("ConfigValidator")
}

func (p *MockPolicy) ConstraintsValidator(ctx envcontext.ProviderCallContext) (constraints.Validator, error) {
	if p.GetConstraintsValidator != nil {
		return p.GetConstraintsValidator()
	}
	return nil, errors.NotImplementedf("ConstraintsValidator")
}

func (p *MockPolicy) StorageProviderRegistry() (storage.ProviderRegistry, error) {
	if p.GetStorageProviderRegistry != nil {
		return p.GetStorageProviderRegistry()
	}
	return nil, errors.NotImplementedf("StorageProviderRegistry")
}

type MockConfigSchemaSource struct {
	CloudName string
}

func (m *MockConfigSchemaSource) ConfigSchema() schema.Fields {
	configSchema := environschema.Fields{
		"providerAttr" + m.CloudName: {
			Type: environschema.Tstring,
		},
	}
	fs, _, err := configSchema.ValidationSchema()
	if err != nil {
		panic(err)
	}
	return fs
}

func (m *MockConfigSchemaSource) ConfigDefaults() schema.Defaults {
	return schema.Defaults{
		"providerAttr" + m.CloudName: "vulch",
	}
}

type MockCredentialService struct {
	Credential *cloud.Credential
}

func (m *MockCredentialService) CloudCredential(ctx stdcontext.Context, id credential.ID) (cloud.Credential, error) {
	if m.Credential == nil {
		return cloud.Credential{}, errors.NotFoundf("credential %q", id)
	}
	return *m.Credential, nil
}

type MockCloudService struct {
	Cloud *cloud.Cloud
}

func (m *MockCloudService) Get(ctx stdcontext.Context, name string) (*cloud.Cloud, error) {
	if m.Cloud == nil {
		return nil, errors.NotFoundf("cloud %q", name)
	}
	return m.Cloud, nil
}
