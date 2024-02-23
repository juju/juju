// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package equinix

import (
	"os"

	"github.com/juju/errors"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
)

type environProviderCredentials struct{}

// CredentialSchemas is part of the environs.ProviderCredentials interface.
func (environProviderCredentials) CredentialSchemas() map[cloud.AuthType]cloud.CredentialSchema {
	return map[cloud.AuthType]cloud.CredentialSchema{
		cloud.AccessKeyAuthType: {
			{
				"project-id",
				cloud.CredentialAttr{
					Description: "Packet project ID",
				},
			}, {
				"api-token",
				cloud.CredentialAttr{
					Description: "Packet API token",
					Hidden:      true,
				},
			},
		},
	}
}

// DetectCredentials is part of the environs.ProviderCredentials interface.
func (e environProviderCredentials) DetectCredentials(cloudName string) (*cloud.CloudCredential, error) {
	type accessKeyValues struct {
		ProjectID string
		AuthToken string
	}
	creds := accessKeyValues{}
	result := cloud.CloudCredential{
		AuthCredentials: make(map[string]cloud.Credential),
	}

	if val, present := os.LookupEnv("METAL_AUTH_TOKEN"); present {
		creds.AuthToken = val
	} else {
		logger.Debugf("METAL_AUTH_TOKEN environment variable not set")
		return nil, errors.NotFoundf("equinix metal auth token")
	}

	if val, present := os.LookupEnv("METAL_PROJECT_ID"); present {
		creds.ProjectID = val
	} else {
		logger.Debugf("METAL_PROJECT_ID environment variable not set")
		return nil, errors.NotFoundf("equinix metal project ID")
	}

	result.AuthCredentials["default"] = cloud.NewCredential(
		cloud.AccessKeyAuthType,
		map[string]string{
			"project-id": creds.ProjectID,
			"api-token":  creds.AuthToken,
		},
	)

	return &result, nil
}

// FinalizeCredential is part of the environs.ProviderCredentials interface.
func (environProviderCredentials) FinalizeCredential(_ environs.FinalizeCredentialContext, args environs.FinalizeCredentialParams) (*cloud.Credential, error) {
	return &args.Credential, nil
}
