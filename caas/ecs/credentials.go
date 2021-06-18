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
func (e providerCredentials) DetectCredentials(string) (*cloud.CloudCredential, error) {
	return nil, errors.NotFoundf("credentials")
}

// FinalizeCredential is part of the environs.ProviderCredentials interface.
func (providerCredentials) FinalizeCredential(_ environs.FinalizeCredentialContext, args environs.FinalizeCredentialParams) (*cloud.Credential, error) {
	return &args.Credential, nil
}

func validateCloudCredential(cred *cloud.Credential) error {
	if cred == nil {
		return errors.NotValidf("missing credential")
	}
	authType := cred.AuthType()
	if authType == "" {
		return errors.NotValidf("missing auth-type")
	}
	if authType != cloud.AccessKeyAuthType {
		return errors.NotSupportedf("%q auth-type", authType)
	}

	credentialAttrs := cred.Attributes()
	if len(credentialAttrs) == 0 {
		return errors.NotValidf("empty credential attributes")
	}
	accessKey := credentialAttrs[credAttrAccessKey]
	if len(accessKey) == 0 {
		return errors.NotValidf("empty %q", credAttrAccessKey)
	}
	secretKey := credentialAttrs[credAttrSecretKey]
	if len(secretKey) == 0 {
		return errors.NotValidf("empty %q", credAttrSecretKey)
	}
	region := credentialAttrs[credAttrRegionKey]
	if len(region) == 0 {
		return errors.NotValidf("empty %q", credAttrRegionKey)
	}
	clusterName := credentialAttrs[credAttrClusterName]
	if clusterName == "" {
		return errors.NotValidf("empty %q", credAttrClusterName)
	}
	return nil
}
