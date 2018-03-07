// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/params"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/state"
)

func CloudToParams(cloud jujucloud.Cloud) params.Cloud {
	authTypes := make([]string, len(cloud.AuthTypes))
	for i, authType := range cloud.AuthTypes {
		authTypes[i] = string(authType)
	}
	regions := make([]params.CloudRegion, len(cloud.Regions))
	for i, region := range cloud.Regions {
		regions[i] = params.CloudRegion{
			Name:             region.Name,
			Endpoint:         region.Endpoint,
			IdentityEndpoint: region.IdentityEndpoint,
			StorageEndpoint:  region.StorageEndpoint,
		}
	}
	return params.Cloud{
		Type:             cloud.Type,
		AuthTypes:        authTypes,
		Endpoint:         cloud.Endpoint,
		IdentityEndpoint: cloud.IdentityEndpoint,
		StorageEndpoint:  cloud.StorageEndpoint,
		Regions:          regions,
		CACertificates:   cloud.CACertificates,
	}
}

func CloudFromParams(cloudName string, p params.Cloud) jujucloud.Cloud {
	authTypes := make([]jujucloud.AuthType, len(p.AuthTypes))
	for i, authType := range p.AuthTypes {
		authTypes[i] = jujucloud.AuthType(authType)
	}
	regions := make([]jujucloud.Region, len(p.Regions))
	for i, region := range p.Regions {
		regions[i] = jujucloud.Region{
			Name:             region.Name,
			Endpoint:         region.Endpoint,
			IdentityEndpoint: region.IdentityEndpoint,
			StorageEndpoint:  region.StorageEndpoint,
		}
	}
	return jujucloud.Cloud{
		Name:             cloudName,
		Type:             p.Type,
		AuthTypes:        authTypes,
		Endpoint:         p.Endpoint,
		IdentityEndpoint: p.IdentityEndpoint,
		StorageEndpoint:  p.StorageEndpoint,
		Regions:          regions,
		CACertificates:   p.CACertificates,
	}
}

// CredentialSchemaGetter describes a function signature that will return a
// a map of credential schemas indexed by auth type, or error,
// for the input cloud name.
type CredentialSchemaGetter func(string) (map[jujucloud.AuthType]jujucloud.CredentialSchema, error)

// CachingCredentialSchemaGetter returns a CredentialSchemaGetter.
// The returned method closes over a cache of such maps that obviates the need
// to go to state for repeat calls for a cloud name.
func CachingCredentialSchemaGetter(accessor state.CloudAccessor) CredentialSchemaGetter {
	schemaCache := make(map[string]map[jujucloud.AuthType]jujucloud.CredentialSchema)
	return func(cloudName string) (map[jujucloud.AuthType]jujucloud.CredentialSchema, error) {
		if s, ok := schemaCache[cloudName]; ok {
			return s, nil
		}
		cloud, err := accessor.Cloud(cloudName)
		if err != nil {
			return nil, err
		}
		provider, err := environs.Provider(cloud.Type)
		if err != nil {
			return nil, err
		}
		schema := provider.CredentialSchemas()
		schemaCache[cloudName] = schema
		return schema, nil
	}
}

// CredentialInfoFromStateCredential generates a ControllerCredentialInfo
// instance from the supplied state credential.
// Passing true for includeSecrets overrides the the Hidden property of
// schema attributes, forcing them to be present in the output.
func CredentialInfoFromStateCredential(
	credential state.Credential,
	includeSecrets bool,
	credentialSchemas CredentialSchemaGetter,
) (params.ControllerCredentialInfo, error) {
	schemas, err := credentialSchemas(credential.Cloud)
	if err != nil {
		return params.ControllerCredentialInfo{}, errors.Trace(err)
	}

	// Filter out the secrets.
	attrs := map[string]string{}
	if s, ok := schemas[jujucloud.AuthType(credential.AuthType)]; ok {
		for _, attr := range s {
			if value, exists := credential.Attributes[attr.Name]; exists {
				if attr.Hidden && !includeSecrets {
					continue
				}
				attrs[attr.Name] = value
			}
		}
	}

	return params.ControllerCredentialInfo{
		Content: params.CredentialContent{
			Name:       credential.Name,
			AuthType:   credential.AuthType,
			Attributes: attrs,
			Cloud:      credential.Cloud,
		},
	}, nil
}
