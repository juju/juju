// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ecs

import (
	"github.com/juju/errors"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
)

const (
	credAttrClusterName = "cluster-name"
	credAttrRegionKey   = "region"
	credAttrAccessKey   = "access-key"
	credAttrSecretKey   = "secret-key"
)

type providerCredentials struct{}

// CredentialSchemas is part of the environs.ProviderCredentials interface.
func (providerCredentials) CredentialSchemas() map[cloud.AuthType]cloud.CredentialSchema {
	return map[cloud.AuthType]cloud.CredentialSchema{
		cloud.AccessKeyAuthType: {
			{
				Name: credAttrClusterName,
				CredentialAttr: cloud.CredentialAttr{
					Description: "The ECS cluster name",
				},
			},
			{
				Name: credAttrRegionKey,
				CredentialAttr: cloud.CredentialAttr{
					Description: "The region that the ECS cluster runs in",
				},
			},
			{
				Name: credAttrAccessKey,
				CredentialAttr: cloud.CredentialAttr{
					Description: "The AWS access key",
				},
			},
			{
				Name: credAttrSecretKey,
				CredentialAttr: cloud.CredentialAttr{
					Description: "The AWS secret key",
					Hidden:      true,
				},
			},
		},
	}
}

// DetectCredentials is part of the environs.ProviderCredentials interface.
func (e providerCredentials) DetectCredentials() (*cloud.CloudCredential, error) {
	return nil, errors.NotFoundf("credentials")
}

// FinalizeCredential is part of the environs.ProviderCredentials interface.
func (providerCredentials) FinalizeCredential(_ environs.FinalizeCredentialContext, args environs.FinalizeCredentialParams) (*cloud.Credential, error) {
	return &args.Credential, nil
}
